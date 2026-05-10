# Runbook: aks-otel-zap

Step-by-step bring-up from zero, with every place the **Azure portal is
unavoidable today** flagged with 🚪. Everything else can be scripted.

For the resource-by-resource explanation of what's being created, read
[AZURE-SETUP.md](./AZURE-SETUP.md) first. For day-2 log-querying recipes, see
[README.md](./README.md#querying-logs).

---

## Prerequisites

- `az` CLI ≥ 2.86 (older releases hit `ValueError: too many values to unpack`
  when creating AKS with `aks-preview`).
- `kubectl`.
- Logged in to the target subscription as a user with rights to create
  resource groups, AKS clusters, ACR, Log Analytics, and Application Insights.

```sh
az login
az account set --subscription 21f320bb-0abe-407e-aba8-480d084c058c
```

All variables live in `infra/env.sh` — override before any step by exporting,
e.g. `LOCATION=eastus2 BASE_NAME=aks-otel-zap-eu ...`.

---

## Step 1 — Register preview features (scripted)

```sh
cd infra
./00-register-features.sh
```

Registers `Microsoft.ContainerService/AzureMonitorAppMonitoringPreview` and
`Microsoft.Insights/OtlpApplicationInsights`, and ensures the `aks-preview`
extension is installed/updated. Re-run until both features report
**Registered** (usually <1 min, occasionally several).

---

## Step 2 — Provision RG + LAW + ACR + AKS (scripted)

```sh
./10-create-cluster.sh
```

Creates:

- Resource group `aks-otel-zap-rg`.
- Log Analytics workspace `aks-otel-zap-law` (Container Insights destination).
- ACR `aksotelzapacr`.
- AKS cluster `aks-otel-zap-aks` with:
  - System-assigned managed identity.
  - Container Insights addon (`--enable-addons monitoring`).
  - Managed Prometheus (`--enable-azure-monitor-metrics`) — required by
    `--enable-opentelemetry-metrics`.
  - App Monitoring preview addon
    (`--enable-azure-monitor-app-monitoring`, `--enable-opentelemetry-logs`,
    `--enable-opentelemetry-metrics`).
  - ACR attached so the kubelet can pull images.

Also pulls kubeconfig (`az aks get-credentials`).

---

## Step 3 — Create OTLP-enabled Application Insights 🚪 (portal)

The ARM property `properties.Features.OpenTelemetry.Enabled = true` is
**silently dropped** by the resource provider today, even on
`apiVersion=2020-02-02-preview`. There is currently no working CLI/ARM way
to set the OTLP toggle. Use the portal.

1. Portal → **Create a resource → Application Insights**.
2. Resource group: `aks-otel-zap-rg`.
3. Name: `aks-otel-zap-app-insights-with-OTLP-support-via-portal` (any name;
   set `APPINSIGHTS_NAME` if you choose a different one).
4. **Enable OTLP Support (Preview)** = **On**.
5. **Use managed workspaces** = **Yes** (creates a hidden LAW under
   `ai_<ai-name>_<guid>_managed`).
6. Region: `centralus`. Create.

Then persist the resource details for the rest of the scripts:

```sh
SKIP_DEPLOY=1 \
APPINSIGHTS_NAME=aks-otel-zap-app-insights-with-OTLP-support-via-portal \
  ./20-create-appinsights.sh
```

`infra/appinsights.json` now holds the connection string and ARM ids.

---

## Step 4 — Onboard the namespace (scripted)

```sh
./30-onboard-namespace.sh
```

Applies `k8s/namespace.yaml` and renders `k8s/instrumentation.yaml` with the
captured connection string. The `Instrumentation` CR with
`autoInstrumentationPlatforms: []` puts the namespace in **autoconfiguration**
mode: the addon's mutating webhook injects `OTEL_EXPORTER_OTLP_*` env vars
into our pods but does **not** inject any SDK.

---

## Step 5 — Wire the AKS namespace to App Insights 🚪 (portal)

> ⚠️ The Instrumentation CR alone is **not** enough. The in-cluster OTLP
> listener on `ama-logs` (port `28331` on the host) only binds once the
> Application Monitoring portal flow attaches AppInsights to the namespace.
> Until then, pods get `dial tcp <node-ip>:28331: connect: connection refused`.

1. Portal → AKS resource `aks-otel-zap-aks` → **Kubernetes resources** →
   **Namespaces** → select `aks-otel-zap`.
2. **Application Monitoring (Preview)** tab → **Configure**.
3. Application Insights: `aks-otel-zap-app-insights-with-OTLP-support-via-portal`.
4. **Instrumentation Type**: **User-configured instrumentation per deployment**
   (autoconfiguration — matches our Instrumentation CR's empty
   `autoInstrumentationPlatforms`).
5. Leave **Perform rollout restart** unchecked. Click **Configure**.

The UI message "Pods in this deployment need to be restarted." is the
expected next-step prompt, not an error.

If `ama-logs` was already running when you did this, restart it so it picks
up the new config and actually opens the OTLP host port:

```sh
kubectl rollout restart daemonset/ama-logs -n kube-system
kubectl rollout status   daemonset/ama-logs -n kube-system
```

Verify the listener is up (run from any node-local view):

```sh
POD=$(kubectl get pods -n kube-system -o name | grep '^pod/ama-logs-' | grep -v rs- | head -1 | cut -d/ -f2)
kubectl exec -n kube-system "$POD" -c ama-logs -- bash -c \
  'for p in 4319 28331; do (echo > /dev/tcp/127.0.0.1/$p) 2>/dev/null && echo "$p OPEN" || echo "$p CLOSED"; done'
```

Expect `4319 OPEN` (container port `amacoreagent` listens on; `28331` maps to
this on the host). `28331 CLOSED` when probed from inside the container is
expected — its only path is the host-port mapping.

---

## Step 6 — Build, push, and deploy the Go service (scripted)

```sh
./40-build-and-deploy.sh
```

- Builds and pushes the image via `az acr build` (no local Docker required).
- Renders `k8s/deployment.yaml` with the resolved ACR image ref and applies
  it. Tags include `latest` and a timestamped immutable tag for rollback.
- Waits for the rollout to converge.

Verify the addon injected OTLP env vars into the pod (these are not visible
via `env` inside the container because they use `$(POD_FIELD)` interpolation;
look at the pod spec):

```sh
kubectl get pod -n aks-otel-zap -l app.kubernetes.io/name=aks-otel-zap \
  -o yaml | grep -A1 'OTEL_EXPORTER_OTLP_'
```

Expect endpoints like `http://$(OTEL_ENDPOINT_NODE_IP):28331/v1/logs`.

---

## Step 7 — Exercise the service and verify ingestion

Produce some logs (see [README.md → Producing logs on demand](./README.md#producing-logs-on-demand)):

```sh
kubectl port-forward -n aks-otel-zap svc/aks-otel-zap 8080:80 &
curl 'http://localhost:8080/log?level=info&msg=hello'
curl 'http://localhost:8080/log?level=warn&msg=watch-out&customer=acme'
```

Wait 1–3 minutes for ingestion, then query (see [README.md → Querying logs](./README.md#querying-logs)):

```sh
# App Insights `traces` (classic projection)
APP_ID=$(jq -r '.properties.AppId' infra/appinsights.json)
az monitor app-insights query --app "$APP_ID" --analytics-query '
  traces
  | where timestamp > ago(15m) and cloud_RoleName == "[aks-otel-zap]/aks-otel-zap"
  | project timestamp, message, severityLevel, customDimensions
  | order by timestamp desc | take 20'

# Managed LAW `OTelLogs` (OTel-native)
WS_ID=$(jq -r '.properties.WorkspaceResourceId' infra/appinsights.json)
WS_GUID=$(az monitor log-analytics workspace show --ids "$WS_ID" --query customerId -o tsv)
az monitor log-analytics query --workspace "$WS_GUID" --analytics-query '
  OTelLogs
  | where TimeGenerated > ago(15m) and ScopeName == "aks-otel-zap"
  | project TimeGenerated, SeverityText, Body, Attributes
  | order by TimeGenerated desc | take 20'
```

---

## Summary: where the portal is unavoidable

| Step | What scripts can do | What still requires the portal |
| ---- | ------------------- | ------------------------------ |
| 1. Feature registration | ✅ Fully scripted | — |
| 2. RG / LAW / ACR / AKS | ✅ Fully scripted | — |
| 3. AppInsights w/ OTLP | ❌ ARM toggle silently dropped | 🚪 Toggle "Enable OTLP Support (Preview)" + "Use managed workspaces" |
| 4. Namespace Instrumentation CR | ✅ Fully scripted | — |
| 5. Cluster ↔ AppInsights wiring | ❌ No CLI surface | 🚪 AKS → Namespaces → Application Monitoring (Preview) → Configure |
| 6. Container build + deploy | ✅ Fully scripted (`az acr build`) | — |
| 7. Verification | ✅ `az monitor ... query` | (Portal Logs blade is more pleasant for ad-hoc) |

Once both preview surfaces stabilize, steps 3 and 5 should collapse into the
existing scripts.

---

## Troubleshooting

### Pod logs show `dial tcp <ip>:28331: connect: connection refused`

The `ama-logs` DaemonSet's OTLP receiver isn't listening. Cause: portal
**Step 5** wasn't performed, or `ama-logs` was already running when it was
performed. Fix: complete Step 5, then `kubectl rollout restart
daemonset/ama-logs -n kube-system`.

### AppInsights `traces` shows nothing but `kubectl logs` is full of entries

- Ingestion lag: wait 1–3 minutes after the first log.
- Wrong AppInsights: confirm `infra/appinsights.json` points at the OTLP one
  (re-run `SKIP_DEPLOY=1 APPINSIGHTS_NAME=... ./20-create-appinsights.sh`).
- Wrong cloud role: filter on `cloud_RoleName startswith "[aks-otel-zap]"`.

### `az aks create` fails with `ValueError: too many values to unpack`

Old `az` CLI bug interacting with `aks-preview`. Upgrade:
`brew upgrade azure-cli` (need ≥ 2.86) and retry.

### `OTelLogs.ResourceAttributes` is empty

Expected with this pipeline today — the addon's collector strips resource
attrs. Service identity is on `ScopeName` instead; per-record k8s.* fields
are inside `Attributes`. See [README.md → B. Log Analytics workspace](./README.md#b-log-analytics-workspace-otel-native-schema).

### Need to nuke and start over

```sh
az group delete -g aks-otel-zap-rg --yes --no-wait
# Also delete the managed LAW the portal created in the hidden RG (purge if needed):
az group delete -g $(az group list --query "[?starts_with(name,'ai_aks-otel-zap-app-insights')].name | [0]" -o tsv) --yes --no-wait
```

Then start from Step 1.
