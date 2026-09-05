# OTLP curl input

This test sends three log records from one OTLP resource through Fluent Bit to
Azure Monitor. The resource contains this collection of attributes:

| Attribute | Expected value |
| --- | --- |
| `test.run.id` | Unique `resource-attributes-<UUID>` value generated for each run |
| `test.resource.dataset` | `multi-record-resource-attributes` |
| `test.resource.region` | `centralus` |
| `test.resource.version` | `1` |

Each log record also has a unique `test.record.id` (`record-1` through
`record-3`) and repeats `test.run.id` as a record attribute. This makes it
possible to match the source records to the ingested rows while independently
checking the resource attributes.

The payload matches the OTLP JSON shape shown in the
[OpenTelemetry file exporter examples](https://opentelemetry.io/docs/specs/otel/protocol/file-exporter/#examples).

## Send the payload

Provision the resources under `azure-resources/az-monitor-otlp-fb/`, then run:

```sh
./send-logs
```

The script prints the unique test run ID and the formatted payload before
sending it. The exact rendered request is also preserved as `payload.json`
beside the template, so the input for the most recent run can be audited after
the command exits.

## Verify `OTelLogs.ResourceAttributes`

Copy the printed run ID into `TestRunId` and run this query in the provisioned
Log Analytics workspace:

```kusto
let TestRunId = "resource-attributes-<UUID>";
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

The result should contain `record-1`, `record-2`, and `record-3`. Every row's
`ResourceAttributes` value should contain the same four attributes from the
table above.

Use this summary query for a single pass/fail result:

```kusto
let TestRunId = "resource-attributes-<UUID>";
let ExpectedRecordCount = 3;
OTelLogs
| where TimeGenerated > ago(1h)
| where tostring(Attributes["test.run.id"]) == TestRunId
| summarize
    RecordCount = count(),
    DistinctRecordCount = dcount(tostring(Attributes["test.record.id"])),
    RowsWithCompleteResourceAttributes = countif(
        tostring(ResourceAttributes["test.run.id"]) == TestRunId
        and tostring(ResourceAttributes["test.resource.dataset"]) == "multi-record-resource-attributes"
        and tostring(ResourceAttributes["test.resource.region"]) == "centralus"
        and tostring(ResourceAttributes["test.resource.version"]) == "1")
| extend Verified =
    RecordCount == ExpectedRecordCount
    and DistinctRecordCount == ExpectedRecordCount
    and RowsWithCompleteResourceAttributes == ExpectedRecordCount
```

`Verified == true` demonstrates that all three log records arrived and that
the complete resource-attribute collection was duplicated into
`OTelLogs.ResourceAttributes` for every record.
