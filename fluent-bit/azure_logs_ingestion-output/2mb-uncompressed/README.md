# Fluent Bit uncompressed 2 MiB chunk repro

This repro tests whether Fluent Bit's `azure_logs_ingestion` output can send a
normal full chunk (approximately 2 MiB) to Azure Monitor without gzip
compression. It uses whichever Fluent Bit binary is selected at runtime.

The YAML uses the `dummy` input to emit 30,000 records as 300 small batches of
100 records. No script or input file generates the records. The small appends
let Fluent Bit fill and divide chunks at its normal soft boundary instead of
injecting one oversized input buffer. The output sends each chunk as one JSON
request. Before the output flush, the runner reads Fluent Bit's local storage
metrics and prints the total buffered size and chunk count. With Fluent Bit
5.1.2, this configuration buffers about 4.5 MiB across three chunks. The two
full chunks serialize to 2,640,001-byte uncompressed JSON requests; the
remaining chunk serializes to 720,001 bytes. Sizes can vary between Fluent Bit
versions, so the runner reports the observed values rather than enforcing them.

## Prerequisites

- Fluent Bit with the `dummy` input and `azure_logs_ingestion` output
- Azure CLI authenticated with `az login`, or an existing service principal
- `curl`
- `jq`
- An existing direct Data Collection Rule created by
  `azure-resources/create-resources`

By default, `send-logs` reads the DCR from:

- `../../../azure-resources/az-monitor-otel-logs-2/dcr.json`

If that artifact doesn't exist, the runner reads
`az-monitor-otel-logs-2-rule` from resource group
`az-monitor-otel-logs-2-rg` with Azure CLI. Override `RESOURCE_GROUP` and
`RULE_NAME` for another DCR.

If `service-principal.json` exists in that directory, the runner uses it. If it
doesn't, the runner gets an Azure Monitor access token from the current Azure
CLI session and exposes it to Fluent Bit through a temporary loopback OAuth
endpoint. The endpoint stops when the test finishes and creates no persistent
Azure credential.

Override `RESOURCE_DIR`, `DCR_PATH`, or `SERVICE_PRINCIPAL_PATH` to use other
artifacts. Set `AUTH_PORT` if loopback port 2022 is unavailable.

## Run

```console
./send-logs
```

The runner prints the selected Fluent Bit version. Set `FLUENT_BIT_BIN` to test
another installed binary:

```console
FLUENT_BIT_BIN=/path/to/fluent-bit ./send-logs
```

The output configuration explicitly sets `compress: false`. The runner prints
the HTTP statuses returned by Azure Monitor and, for rejected requests, Fluent
Bit's `retrying payload bytes=...` diagnostic.

- **PASS** means every observed chunk received an HTTP 2xx response.
- **FAIL** means at least one chunk was rejected or no HTTP 2xx response was
  observed. An HTTP 413 with a payload near 2 MiB demonstrates that the plugin
  sent a full chunk as one uncompressed request and exceeded Azure's request
  size limit.
- **BLOCKED** with HTTP 401 or 403 means the request reached Azure, but the
  selected identity lacks the DCR ingestion permission needed to test the
  payload-size behavior.

Set `KEEP_LOG=true` to retain the complete Fluent Bit debug log for a bug
report. Set `RUN_SECONDS` to change the default 15-second timeout, or
`HTTP_PORT` if port 2021 is unavailable.
