#!/usr/bin/env bash
# Provision RG + Log Analytics workspace + AKS cluster (Linux only) with
# Container Insights and Application Monitoring (preview) support enabled.
# Also creates an ACR and attaches it to the cluster so we can push the Go
# service image without managing pull secrets.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=env.sh
source "$SCRIPT_DIR/env.sh"

echo "==> Selecting subscription $SUBSCRIPTION_ID"
az account set --subscription "$SUBSCRIPTION_ID"

echo "==> Creating resource group $RESOURCE_GROUP in $LOCATION"
az group create --name "$RESOURCE_GROUP" --location "$LOCATION" >/dev/null

echo "==> Creating Log Analytics workspace $LAW_NAME (for Container Insights)"
LAW_ID=$(az monitor log-analytics workspace create \
  --resource-group "$RESOURCE_GROUP" \
  --workspace-name "$LAW_NAME" \
  --location "$LOCATION" \
  --query id -o tsv)
echo "    LAW_ID=$LAW_ID"

echo "==> Creating ACR $ACR_NAME"
az acr create \
  --resource-group "$RESOURCE_GROUP" \
  --name "$ACR_NAME" \
  --sku Basic \
  --location "$LOCATION" >/dev/null

echo "==> Creating AKS cluster $AKS_NAME (Linux node pool, system-assigned MI)"
# --enable-addons monitoring + --workspace-resource-id wires Container Insights.
# --enable-azure-monitor-metrics enables Managed Prometheus (creates/uses a
#   default Azure Monitor workspace in DefaultResourceGroup-<region>) and is a
#   prerequisite for --enable-opentelemetry-metrics.
# --enable-azure-monitor-app-monitoring turns on autoinstrumentation support
#   (gated by AzureMonitorAppMonitoringPreview).
# --enable-opentelemetry-logs / --enable-opentelemetry-metrics turn on the
#   "data collection from vendor-neutral OpenTelemetry SDKs (Preview)" toggle
#   from the Azure portal — required for autoconfiguration to inject
#   OTEL_EXPORTER_OTLP_* into our Go pods.
az aks create \
  --resource-group "$RESOURCE_GROUP" \
  --name "$AKS_NAME" \
  --location "$LOCATION" \
  --node-count 2 \
  --node-vm-size Standard_DS2_v2 \
  --enable-managed-identity \
  --enable-addons monitoring \
  --workspace-resource-id "$LAW_ID" \
  --enable-azure-monitor-metrics \
  --enable-azure-monitor-app-monitoring \
  --enable-opentelemetry-logs \
  --enable-opentelemetry-metrics \
  --attach-acr "$ACR_NAME" \
  --generate-ssh-keys \
  --only-show-errors

echo "==> Fetching kubeconfig"
az aks get-credentials \
  --resource-group "$RESOURCE_GROUP" \
  --name "$AKS_NAME" \
  --overwrite-existing

echo "==> Cluster nodes:"
kubectl get nodes

cat <<NOTE

AKS cluster is up and Container Insights is wired to $LAW_NAME.
Next: ./20-create-appinsights.sh to provision the OTLP-enabled Application
Insights resource.
NOTE
