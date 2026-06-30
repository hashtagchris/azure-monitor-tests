# azure-monitor-tests

Create a workspace and DCR, and send logs to it.

## Variants

### `azure_logs_ingestion` output (custom-stream Logs Ingestion API)

Uses Fluent Bit's [`azure_logs_ingestion`](https://docs.fluentbit.io/manual/pipeline/outputs/azure_logs_ingestion)
plugin against a custom-stream DCR (`Custom-MyFluentBitLogs` →
`Microsoft-OTel-Logs`). DCRs are `kind: Direct` (no DCE).

- Resources: `azure-resources/az-monitor-otel-logs-2/` (and earlier `az-monitor-*`
  experiments).
- Configs: `fluent-bit/azure_logs_ingestion-output/{dummy-input,tail-input}/`.

### `opentelemetry` output (native OTLP ingestion, preview)

Uses Fluent Bit's [`opentelemetry`](https://docs.fluentbit.io/manual/pipeline/outputs/opentelemetry)
plugin against Azure Monitor's native OTLP/HTTP logs endpoint, which requires
a Data Collection **Endpoint** (DCE) in addition to the DCR. The DCR uses the
predefined `Microsoft-OTel-Logs` stream via `directDataSources.otelLogs`.
Authentication uses the plugin's built-in OAuth 2.0 client credentials flow
against Microsoft Entra (scope `https://monitor.azure.com/.default`).

- Resources: `azure-resources/az-monitor-otlp-fb/` (provisioned via
  `arm-template.json` + `create-resources`).
- Configs: `fluent-bit/opentelemetry-output/{dummy-input,tail-input}/`.

Logs land in the OTel logs schema in Log Analytics (e.g. the `AppTraces` /
OTel-Logs table; check the workspace after first ingestion).

### Auxiliary and Analytics custom tables

Creates two Log Analytics workspaces, each with three identically shaped custom
tables on the Auxiliary table plan and two on the Analytics table plan, plus a
script to populate them with dummy logs.

- Resources and scripts: `azure-resources/auxiliary-tables/`.
