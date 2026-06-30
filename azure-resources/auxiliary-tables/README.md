# Auxiliary and Analytics custom tables

This scenario provisions two Log Analytics workspaces. Each workspace has three
custom tables that use the Auxiliary table plan and two custom tables that use
the Analytics table plan. All tables share an identical schema. The scenario
also includes a script that sends dummy logs to every table through the Logs
Ingestion API.

## Prerequisites

- Azure CLI authenticated with `az login`
- `jq`, `curl`, and `uuidgen`
- Permissions to create resource groups, Log Analytics workspaces, custom tables,
  Data Collection Endpoints, Data Collection Rules, service principals, and role
  assignments

Auxiliary tables must be created as new tables. Azure Monitor doesn't support
switching an existing table to the Auxiliary plan.

## Provision resources

```bash
./provision
```

By default, the script creates:

- Resource group: `az-monitor-aux-tables-rg`
- Workspaces: `az-monitor-aux-tables-law-1`, `az-monitor-aux-tables-law-2`
- Auxiliary tables in each workspace: `AuxLogsA_CL`, `AuxLogsB_CL`,
  `AuxLogsC_CL`
- Analytics tables in each workspace: `AnalyticsLogsA_CL`, `AnalyticsLogsB_CL`
- One Data Collection Endpoint and one Data Collection Rule per workspace
- One saved Log Analytics function per workspace: `LogsByRequestId(requestId:string)`
- One service principal scoped to the DCRs with `Monitoring Metrics Publisher`

Artifacts are written to `./az-monitor-aux-tables/`, including `config.json`,
DCE/DCR responses, table payloads, function payloads, and
`service-principal.json`.

Configuration overrides:

```bash
SUBSCRIPTION_ID="00000000-0000-0000-0000-000000000000" \
BASE_NAME="my-aux-test" \
LOCATION="eastus" \
WORKSPACE_NAMES="my-aux-law-1,my-aux-law-2" \
AUXILIARY_TABLE_NAMES="MyAuxA_CL,MyAuxB_CL,MyAuxC_CL" \
ANALYTICS_TABLE_NAMES="MyAnalyticsA_CL,MyAnalyticsB_CL" \
FUNCTION_ALIAS="LogsByRequestId" \
TOTAL_RETENTION_IN_DAYS="365" \
./provision
```

The saved function uses the first Analytics table and first Auxiliary table from
the configured table lists. With the default names, the function query is:

```kusto
union AnalyticsLogsA_CL, AuxLogsA_CL
| where RequestID == requestId
```

## Populate dummy logs

```bash
./populate-dummy-logs
```

The script reads `./az-monitor-aux-tables/config.json` and
`./az-monitor-aux-tables/service-principal.json`, requests an Azure Monitor token
with the service principal, and posts dummy records to every workspace/table
pair. It generates one shared set of `RequestID` values per run, so
`SequenceNumber == 1` has the same `RequestID` in every table and workspace,
`SequenceNumber == 2` has the same `RequestID` in every table and workspace, and
so on.

The DCR input stream for each custom table omits the `_CL` suffix, while
`outputStream` targets the actual table name. For example, records for
`AnalyticsLogsA_CL` are posted to stream `Custom-AnalyticsLogsA`, and the DCR
routes that stream to `outputStream: Custom-AnalyticsLogsA_CL`.

Configuration overrides:

```bash
ROWS_PER_TABLE="25" \
BATCH_SIZE="10" \
ARTIFACT_DIR="./az-monitor-aux-tables" \
./populate-dummy-logs
```

## Table schema

Every Auxiliary and Analytics table uses this schema:

| Column | Type |
| --- | --- |
| `TimeGenerated` | `dateTime` |
| `RequestID` | `string` |
| `TableName` | `string` |
| `Message` | `string` |
| `SeverityText` | `string` |
| `Source` | `string` |
| `SequenceNumber` | `int` |

Auxiliary tables don't support `dynamic` columns, so the shared schema uses only
scalar types.

## Query examples

Run these in each workspace after ingestion:

```kusto
union AuxLogsA_CL, AuxLogsB_CL, AuxLogsC_CL, AnalyticsLogsA_CL, AnalyticsLogsB_CL
| summarize Rows=count(), Requests=dcount(RequestID) by TableName
| order by TableName asc
```

```kusto
AuxLogsA_CL
| take 10
```

```kusto
union AuxLogsA_CL, AuxLogsB_CL, AuxLogsC_CL, AnalyticsLogsA_CL, AnalyticsLogsB_CL
| summarize Tables=make_set(TableName), Rows=count() by RequestID, SequenceNumber
| order by SequenceNumber asc
```

```kusto
LogsByRequestId("7e99d35b-50c4-4690-b061-f9fb6eed2423")
```
