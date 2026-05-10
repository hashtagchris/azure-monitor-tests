#!/usr/bin/env bash
# Register the preview features and providers required by the AKS OTLP /
# Application Insights pipeline described at:
#   https://learn.microsoft.com/en-us/azure/azure-monitor/containers/kubernetes-open-protocol
#
# Idempotent. Safe to re-run; registrations stick to the subscription.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=env.sh
source "$SCRIPT_DIR/env.sh"

echo "==> Checking az login..."
az account show >/dev/null || { echo "Not logged in. Run 'az login' first." >&2; exit 1; }

echo "==> Selecting subscription $SUBSCRIPTION_ID"
az account set --subscription "$SUBSCRIPTION_ID"

echo "==> Ensuring aks-preview extension is installed and up to date"
az extension add --name aks-preview --only-show-errors >/dev/null 2>&1 || true
az extension update --name aks-preview --only-show-errors >/dev/null 2>&1 || true

echo "==> Registering Microsoft.ContainerService/AzureMonitorAppMonitoringPreview"
az feature register \
  --namespace Microsoft.ContainerService \
  --name AzureMonitorAppMonitoringPreview >/dev/null

echo "==> Registering Microsoft.Insights/OtlpApplicationInsights"
az feature register \
  --namespace Microsoft.Insights \
  --name OtlpApplicationInsights >/dev/null

echo "==> Re-registering providers so feature flags take effect"
az provider register --namespace Microsoft.ContainerService >/dev/null
az provider register --namespace Microsoft.Insights >/dev/null

echo "==> Current feature state:"
az feature list -o table --query "[?name=='Microsoft.ContainerService/AzureMonitorAppMonitoringPreview' || name=='Microsoft.Insights/OtlpApplicationInsights'].{Name:name,State:properties.state}"

cat <<'NOTE'

Feature registrations can take several minutes to move from "Registering" to
"Registered". Re-run this script (or `az feature show ...`) until both show
"Registered" before continuing with 10-create-cluster.sh.
NOTE
