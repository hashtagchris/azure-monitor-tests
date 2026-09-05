# Direct Azure Monitor OTLP/HTTP test

This standalone test renders OTLP logs as auditable JSON, encodes the request
as OTLP protobuf, and sends it directly to Azure Monitor's native OTLP
endpoint. It does not install, configure, or run Fluent Bit.

The test provisions its own:

- Log Analytics workspace
- Data Collection Endpoint (DCE)
- Data Collection Rule (DCR) with a `Microsoft-OTel-Logs` direct data source
- Microsoft Entra service principal scoped to the DCR with
  `Monitoring Metrics Publisher`

## Prerequisites

- Azure CLI authenticated with `az login`
- `curl`
- Go
- `jq`
- Python 3
- Permission to create resource groups, deployments, role assignments, and
  Microsoft Entra applications/service principals

The scripts default to the active Azure subscription and `centralus`. Override
the defaults with environment variables:

```sh
SUBSCRIPTION_ID=<subscription-id> \
LOCATION=centralus \
BASE_NAME=az-monitor-otlp-direct \
./provision
```

`BASE_NAME` is used to derive all resource names. Individual names can also be
overridden with `RESOURCE_GROUP`, `WORKSPACE_NAME`, `DCE_NAME`, `DCR_NAME`, and
`SP_NAME`.

## Provision

```sh
./provision
```

Generated resource snapshots and credentials are stored under `artifacts/`,
which is ignored by Git. `artifacts/service-principal.json` contains a client
secret, is restricted to the current user, and must not be shared or committed.
Re-running `./provision` reuses that service principal and restores its DCR
role assignment if needed. If the artifact is missing but an Entra application
with the same name exists, provisioning stops rather than creating a duplicate.

## Send directly to Azure Monitor

```sh
./send-logs
```

The sender:

1. Generates a unique test run ID and nanosecond timestamp.
2. Renders and validates `artifacts/payload.json`.
3. Prints the exact JSON payload and direct Azure Monitor endpoint.
4. Deterministically encodes that JSON request as OTLP protobuf in
   `artifacts/payload.pb` and prints its SHA-256 digest.
5. Acquires a token from Microsoft Entra using the client-credentials flow and
   the `https://monitor.azure.com/.default` scope.
6. Posts the protobuf body with `Content-Type: application/x-protobuf`
   directly to:

   ```text
   <DCE>/dataCollectionRules/<immutable-DCR-id>/streams/Microsoft-OTLP-Logs/otlp/v1/logs
   ```

7. Retries temporary `401`, `403`, `429`, or `503` responses while a new role
   assignment propagates or the endpoint recovers.
8. Fails on any other non-2xx response and preserves the response body under
   `artifacts/`.

The sender defaults to 30 attempts with 10 seconds between retryable responses.
Override these values with `MAX_SEND_ATTEMPTS` and `RETRY_DELAY_SECONDS`.

Azure Monitor's native OTLP/HTTP endpoint expects protobuf encoding. Sending
the JSON fixture directly with `Content-Type: application/json` returns HTTP
415. The JSON remains the auditable source payload, and the preserved protobuf
file plus printed digest identify the exact bytes sent.

The payload contains three records under one OTLP resource. Each record repeats
the run ID in `Attributes` for correlation, while the resource contains this
shared collection:

| Resource attribute | Expected value |
| --- | --- |
| `test.run.id` | Generated `direct-resource-attributes-<UUID>` value |
| `test.resource.dataset` | `multi-record-resource-attributes` |
| `test.resource.transport` | `direct-otlp-http` |
| `test.resource.region` | `centralus` |
| `test.resource.version` | `1` |

## Verify `OTelLogs.ResourceAttributes`

Open the URL printed by `./provision`, or use the workspace identified in
`artifacts/test-environment.json`. Replace the value below with the run ID
printed by `./send-logs`.

Inspect all records without filtering on `ResourceAttributes`:

```kusto
let TestRunId = "direct-resource-attributes-<UUID>";
OTelLogs
| where TimeGenerated > ago(1h)
| where tostring(Attributes["test.run.id"]) == TestRunId
| project
    TimeGenerated,
    RecordId = tostring(Attributes["test.record.id"]),
    Body,
    ResourceAttributes
| order by RecordId asc
```

The result should contain `record-1`, `record-2`, and `record-3`. Every row
should contain the same five resource attributes.

Use this query for a compact verification result:

```kusto
let TestRunId = "direct-resource-attributes-<UUID>";
let ExpectedRecordCount = 3;
OTelLogs
| where TimeGenerated > ago(1h)
| where tostring(Attributes["test.run.id"]) == TestRunId
| summarize
    RecordCount = count(),
    DistinctRecordCount = count_distinct(tostring(Attributes["test.record.id"])),
    RowsWithCompleteResourceAttributes = countif(
        tostring(ResourceAttributes["test.run.id"]) == TestRunId
        and tostring(ResourceAttributes["test.resource.dataset"]) == "multi-record-resource-attributes"
        and tostring(ResourceAttributes["test.resource.transport"]) == "direct-otlp-http"
        and tostring(ResourceAttributes["test.resource.region"]) == "centralus"
        and tostring(ResourceAttributes["test.resource.version"]) == "1")
| extend Verified =
    RecordCount == ExpectedRecordCount
    and DistinctRecordCount == ExpectedRecordCount
    and RowsWithCompleteResourceAttributes == ExpectedRecordCount
```

`Verified == true` proves the direct endpoint produced all three rows and
duplicated the complete resource-attribute collection into
`OTelLogs.ResourceAttributes` for each record.

## Cleanup

Read the resource group from the generated environment file and delete it:

```sh
RESOURCE_GROUP=$(jq -r '.resourceGroup' artifacts/test-environment.json)
az group delete --name "$RESOURCE_GROUP" --yes
```

Deleting the resource group does not delete the Microsoft Entra application.
Delete that separately when the test environment is no longer needed:

```sh
APP_ID=$(jq -r '.appId' artifacts/service-principal.json)
az ad app delete --id "$APP_ID"
```
