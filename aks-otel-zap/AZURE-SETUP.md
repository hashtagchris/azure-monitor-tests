# Azure setup overview

This document narrates the Azure-side setup performed by the scripts in
`infra/`. It mirrors the AKS OTLP preview onboarding flow from
[Microsoft Learn — Kubernetes Open Protocol](https://learn.microsoft.com/en-us/azure/azure-monitor/containers/kubernetes-open-protocol).

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Subscription                                                            │
│                                                                          │
│  ┌────────── Resource group: aks-otel-zap-rg ──────────────────────────┐ │
│  │                                                                    │ │
│  │  Log Analytics workspace (aks-otel-zap-law)                        │ │
│  │     ▲                                                              │ │
│  │     │ Container Insights metrics + logs                            │ │
│  │     │                                                              │ │
│  │  AKS cluster (aks-otel-zap-aks)                                    │ │
│  │     • Linux node pool only (Windows unsupported by preview)        │ │
│  │     • System-assigned managed identity                             │ │
│  │     • Container Insights addon → LAW above                         │ │
│  │     • Application Monitoring preview addon                         │ │
│  │     • Attached ACR (aksotelzapacr) for image pulls                 │ │
│  │     │                                                              │ │
│  │     │ OTLP/HTTP from instrumented pods                             │ │
│  │     ▼                                                              │ │
│  │  Application Insights (aks-otel-zap-ai, OTLP enabled)              │ │
│  │     • Managed workspace mode                                       │ │
│  │     • Backed by Azure Monitor workspace (aks-otel-zap-amw)         │ │
│  │       — distinct from the LAW used by Container Insights           │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
```

## Step 1 — Register preview features (`00-register-features.sh`)

Both subscription-level features below must reach **Registered** before AKS or
App Insights will accept the preview toggles.

| Namespace | Feature | Why |
| --- | --- | --- |
| `Microsoft.ContainerService` | `AzureMonitorAppMonitoringPreview` | Gates `--enable-azure-monitor-app-monitoring` on AKS, which deploys the in-cluster Azure Monitor OTel collector + admission webhook for autoinstrumentation/autoconfiguration. |
| `Microsoft.Insights` | `OtlpApplicationInsights` | Gates the **Enable OTLP Support (Preview)** flag on Application Insights so it can accept OTLP-shaped logs/metrics/traces. |

The script also installs/updates the `aks-preview` Azure CLI extension, which
is what surfaces the new `az aks` flags. Re-registration of the providers
(`Microsoft.ContainerService`, `Microsoft.Insights`) is required so the
feature flag actually flips on.

## Step 2 — Provision the cluster (`10-create-cluster.sh`)

Creates, in order:

1. **Resource group** `aks-otel-zap-rg` in `centralus`.
2. **Log Analytics workspace** `aks-otel-zap-law` — destination for Container
   Insights (cluster/node/pod infra telemetry). Per the docs, this workspace
   must **not** be reused for Application Insights ingestion in step 3.
3. **Azure Container Registry** `aksotelzapacr` (Basic SKU).
4. **AKS cluster** `aks-otel-zap-aks` with:
   - System-assigned managed identity (used by add-ons; no service principal).
   - `--enable-addons monitoring --workspace-resource-id <LAW>` → Container
     Insights points at the workspace from step 2.
   - `--enable-azure-monitor-app-monitoring` → Application Monitoring preview
     addon. This installs the Azure Monitor in-cluster OTel collector and the
     mutating webhook that injects OTLP env vars / autoinstrumentation
     sidecars into onboarded namespaces.
   - `--attach-acr aksotelzapacr` → grants the cluster's kubelet identity
     `AcrPull` on the registry, so deployments can pull our image without a
     pull secret.
5. Pulls kubeconfig via `az aks get-credentials` for local `kubectl` use.

## Step 3 — Create the OTLP-enabled Application Insights (`20-create-appinsights.sh`)

Deploys `arm-appinsights-otlp.json`, which provisions:

1. **Azure Monitor workspace** `aks-otel-zap-amw` (`Microsoft.Monitor/accounts`) —
   the managed workspace that backs the Application Insights resource. This is
   intentionally separate from `aks-otel-zap-law` so application telemetry is
   isolated from infrastructure telemetry, as the docs require.
2. **Application Insights component** `aks-otel-zap-ai` with:
   - `kind: web`, `Application_Type: web`.
   - `WorkspaceResourceId` pointing at the Azure Monitor workspace above
     (managed-workspaces mode).
   - `Features.OpenTelemetry.Enabled = true` — the ARM equivalent of the
     portal's **Enable OTLP Support (Preview)** toggle.

The script then writes `appinsights.json` (the resource document, including
the `connectionString`). The connection string is what the Application
Monitoring addon hands to onboarded pods via the `OTEL_EXPORTER_OTLP_*` env
vars; the application itself never has to know the DCE/DCR addresses.

If ARM rejects the OTLP toggle (the property name has been moving while the
preview is active), the script prints a portal-fallback path: create the
Application Insights resource manually with the OTLP toggle on, then re-run
the script with `SKIP_DEPLOY=1` to capture `appinsights.json`.

## What's *not* set up by these scripts (yet)

- **Namespace onboarding** — the `ApplicationMonitoring` custom resource that
  binds a Kubernetes namespace to the Application Insights resource above and
  selects autoconfiguration vs. autoinstrumentation. This will live in
  `infra/30-onboard-namespace.sh` and is a precondition for the addon to
  inject OTLP env vars into the Go service's pods.
- **Workload resources** — the Go service Deployment/Service/Namespace
  manifests under `k8s/`, and the container image under `app/` + ACR push.

## Identity & auth summary

- **AKS data plane → ACR**: handled by `--attach-acr` (kubelet identity gets
  `AcrPull`). No image pull secret needed.
- **Pods → Application Insights**: the in-cluster Azure Monitor OTel collector
  performs the authenticated upload to App Insights using the cluster's
  Application Monitoring addon identity. Workload pods only need to point
  `OTEL_EXPORTER_OTLP_ENDPOINT` at the in-cluster collector (which the addon
  injects automatically when the namespace is onboarded).
- **Operator → Azure**: `az login` against the VS Enterprise subscription
  `21f320bb-0abe-407e-aba8-480d084c058c` is sufficient; no service principal
  is created in this folder.

## Naming / regions

All defaults live in `infra/env.sh`. Override by exporting before invoking a
script, e.g. `LOCATION=eastus2 BASE_NAME=aks-otel-zap-eu ./10-create-cluster.sh`.
