# aks-otel-zap

Stand up an AKS cluster running a small Go service that uses Uber zap and
exports logs over OTLP to Azure Monitor / Application Insights using the
AKS-native preview pipeline (no Fluent Bit, no custom collector).

Reference: <https://learn.microsoft.com/en-us/azure/azure-monitor/containers/kubernetes-open-protocol>

For a narrative walkthrough of what each provisioning script does and how the
Azure resources fit together, see [AZURE-SETUP.md](./AZURE-SETUP.md).

For the step-by-step bring-up flow (including the two places the Azure portal
is unavoidable today), see [RUNBOOK.md](./RUNBOOK.md).

## Resource layout

- `infra/env.sh` — shared variables (subscription, region, names). Override by
  exporting any of them before invoking a script.
- `infra/00-register-features.sh` — registers the
  `Microsoft.ContainerService/AzureMonitorAppMonitoringPreview` and
  `Microsoft.Insights/OtlpApplicationInsights` preview features and re-registers
  the providers.
- `infra/10-create-cluster.sh` — RG, Log Analytics workspace, ACR, and AKS
  cluster with Container Insights + Managed Prometheus + App Monitoring preview
  + OTel logs/metrics. Attaches the ACR.
- `infra/arm-appinsights-otlp.json` + `infra/20-create-appinsights.sh` —
  Application Insights with a dedicated LAW. **OTLP toggle is silently dropped
  by ARM today**; this script's portal-fallback path (`SKIP_DEPLOY=1`) is the
  one we actually used.
- `infra/30-onboard-namespace.sh` — applies `k8s/namespace.yaml` and renders
  `k8s/instrumentation.yaml` with the AppInsights connection string, putting
  the namespace in autoconfiguration mode.
- `infra/40-build-and-deploy.sh` — `az acr build` → push → render
  `k8s/deployment.yaml` with the resolved image ref → `kubectl apply`.
- `app/` — Go service (zap → otelzap → otlploghttp) + Dockerfile.
- `k8s/` — namespace, Instrumentation CR template, Deployment + Service.

## Bring-up order

```sh
cd infra
./00-register-features.sh
./10-create-cluster.sh
./20-create-appinsights.sh         # or create the resource in the portal with "Enable OTLP Support (Preview)" = ON, then re-run with SKIP_DEPLOY=1
./30-onboard-namespace.sh
./40-build-and-deploy.sh
```

After step 4, in the portal: **AKS cluster → Kubernetes resources → Namespaces →
`aks-otel-zap` → Application Monitoring (Preview) → pick the OTLP AppInsights
→ "User-configured instrumentation per deployment" → Configure**. This is what
actually starts the in-cluster OTLP listener on `ama-logs` (port `28331` host).
If pods still see `connection refused` afterwards, restart `ama-logs`:

```sh
kubectl rollout restart daemonset/ama-logs -n kube-system
kubectl rollout restart deployment/aks-otel-zap -n aks-otel-zap
```

## Producing logs on demand

The service is `ClusterIP`-only. Easiest options:

### Port-forward from your laptop

```sh
kubectl port-forward -n aks-otel-zap svc/aks-otel-zap 8080:80
# in another terminal:
curl 'http://localhost:8080/log?level=info&msg=hello-from-laptop'
curl 'http://localhost:8080/log?level=warn&msg=oops&customer=acme'
curl 'http://localhost:8080/log?level=error&msg=boom'
```

### One-shot from an in-cluster pod (no port-forward)

```sh
kubectl run curltmp --rm -i --restart=Never --image=curlimages/curl:latest -n aks-otel-zap -- \
  curl -s 'http://aks-otel-zap/log?level=warn&msg=hello-from-cluster&customer=acme'
```

Query parameters:

| Param   | Values                              | Notes                                                                 |
| ------- | ----------------------------------- | --------------------------------------------------------------------- |
| `level` | `debug` \| `info` \| `warn` \| `error` | Maps to zap level → OTel severity. Defaults to `info`.                |
| `msg`   | any string                          | Log message. Defaults to `"log endpoint hit"`.                        |
| any other | any string                        | Attached as a structured field named `query.<param>` in `customDimensions`. |

The service also emits a `heartbeat` INFO log every 15 minutes
(override with `HEARTBEAT_INTERVAL=1m` in the Deployment env).

## Querying logs

Service identity in both views:

- Cloud role: `[aks-otel-zap]/aks-otel-zap` (AppInsights `cloud_RoleName`)
- Service name: `aks-otel-zap` (OTel `ServiceName`)

### A. Application Insights (classic schema)

Use the **`traces`** table. Run from the portal: Application Insights resource
→ **Logs**.

```kusto
traces
| where timestamp > ago(1h) and cloud_RoleName == "[aks-otel-zap]/aks-otel-zap"
| project timestamp, message, severityLevel, customDimensions
| order by timestamp desc
```

Equivalent CLI (requires `--app` = AppInsights `AppId` from
`infra/appinsights.json`):

```sh
APP_ID=$(jq -r '.properties.AppId' infra/appinsights.json)
az monitor app-insights query \
  --app "$APP_ID" \
  --analytics-query 'traces
    | where timestamp > ago(1h) and cloud_RoleName == "[aks-otel-zap]/aks-otel-zap"
    | project timestamp, message, severityLevel, customDimensions
    | order by timestamp desc
    | take 50'
```

`severityLevel` mapping: `0`=verbose/info, `1`=info (some SDKs), `2`=warning,
`3`=error, `4`=critical.

### B. Log Analytics workspace (OTel-native schema)

The OTLP-enabled AppInsights is backed by a managed LAW (auto-created, in a
hidden RG named `ai_<ai-name>_<guid>_managed`). The OTel-shaped data lives in
the **`OTelLogs`** table.

Filter on `ScopeName == "aks-otel-zap"` — that's the logger name our Go service
sets via `otelzap.NewCore(serviceName, ...)`. `ResourceAttributes` is empty
in this pipeline (the addon's collector strips resource attrs), but service
identity comes through as `ScopeName`, and per-record k8s attrs are in
`Attributes`.

```kusto
OTelLogs
| where TimeGenerated > ago(1h) and ScopeName == "aks-otel-zap"
| project TimeGenerated, SeverityText, Body, Attributes
| order by TimeGenerated desc
```

Open from the portal: search the global resource picker for the workspace name
captured in `infra/appinsights.json` at `.properties.WorkspaceResourceId`
(starts with `managed-aks-otel-zap-...-ws`), then **Logs**.

Equivalent CLI (uses the LAW's customer/GUID id, not the ARM resource id):

```sh
WS_ID=$(jq -r '.properties.WorkspaceResourceId' infra/appinsights.json)
WS_GUID=$(az monitor log-analytics workspace show --ids "$WS_ID" --query customerId -o tsv)
az monitor log-analytics query \
  --workspace "$WS_GUID" \
  --analytics-query 'OTelLogs
    | where TimeGenerated > ago(1h) and ScopeName == "aks-otel-zap"
    | project TimeGenerated, SeverityText, Body, Attributes
    | order by TimeGenerated desc
    | take 50'
```

Other useful queries:

```kusto
// Severity histogram for our service
OTelLogs
| where TimeGenerated > ago(1d) and ScopeName == "aks-otel-zap"
| summarize count() by SeverityText, bin(TimeGenerated, 5m)
| render timechart

// Just the heartbeats (sanity check that the pod is alive)
OTelLogs
| where TimeGenerated > ago(2h) and ScopeName == "aks-otel-zap" and Body == "heartbeat"
| project TimeGenerated, seq=Attributes["heartbeat.seq"], uptime=Attributes["process.uptime"]
```

### C. kubectl (raw zap output)

For comparison with what the pod actually emitted:

```sh
kubectl logs -n aks-otel-zap deploy/aks-otel-zap --tail=50 -f
```

The Go service tees zap entries to stdout **and** the OTel logs SDK, so
`kubectl logs` should always match what shows up in App Insights once
ingestion catches up (1–3 minutes typical).
