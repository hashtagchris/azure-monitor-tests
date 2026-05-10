#!/usr/bin/env bash
# Create an Azure Monitor workspace + OTLP-enabled Application Insights
# resource (managed-workspaces). This is the "managed workspace" /
# "Enable OTLP Support (Preview)" combination from:
#   https://learn.microsoft.com/en-us/azure/azure-monitor/containers/kubernetes-open-protocol
#
# IMPORTANT: per the docs the Azure Monitor workspace used here MUST be
# different from the Log Analytics workspace wired to Container Insights in
# 10-create-cluster.sh.
#
# The OTLP toggle (`Features.OpenTelemetry.Enabled`) is preview and the ARM
# property name has been moving. If `az deployment group create` rejects the
# template, follow the "Portal fallback" steps printed at the end and create
# the AppInsights resource by hand, then re-run this script with
# SKIP_DEPLOY=1 to record outputs.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=env.sh
source "$SCRIPT_DIR/env.sh"

echo "==> Selecting subscription $SUBSCRIPTION_ID"
az account set --subscription "$SUBSCRIPTION_ID"

if [[ "${SKIP_DEPLOY:-0}" != "1" ]]; then
  echo "==> Deploying ARM template (Azure Monitor workspace + OTLP-enabled AppInsights)"
  DEPLOYMENT_NAME="${BASE_NAME}-appinsights-$(date +%s)"
  az deployment group create \
    --resource-group "$RESOURCE_GROUP" \
    --name "$DEPLOYMENT_NAME" \
    --template-file "$SCRIPT_DIR/arm-appinsights-otlp.json" \
    --parameters \
      location="$LOCATION" \
      appInsightsLogAnalyticsWorkspaceName="$AI_LAW_NAME" \
      applicationInsightsName="$APPINSIGHTS_NAME" >/dev/null
fi

echo "==> Persisting AppInsights details to $SCRIPT_DIR/appinsights.json"
az resource show \
  --resource-group "$RESOURCE_GROUP" \
  --name "$APPINSIGHTS_NAME" \
  --resource-type Microsoft.Insights/components \
  > "$SCRIPT_DIR/appinsights.json"

CONN_STRING=$(jq -r '.properties.ConnectionString' "$SCRIPT_DIR/appinsights.json")
APPINSIGHTS_ID=$(jq -r '.id' "$SCRIPT_DIR/appinsights.json")

echo "==> AppInsights resource id: $APPINSIGHTS_ID"
echo "==> Connection string (also in appinsights.json):"
echo "    $CONN_STRING"

cat <<NOTE

If the ARM deployment failed because the OTLP toggle is not yet exposed to
ARM in this region, do the following Portal fallback:
  1. In the portal, create an Application Insights resource named
     '$APPINSIGHTS_NAME' in resource group '$RESOURCE_GROUP'.
  2. Toggle "Enable OTLP Support (Preview)" = ON.
  3. Set "Use managed workspaces" = Yes (or pick an existing Log Analytics
     workspace that is NOT $LAW_NAME — Container Insights' LAW must stay
     separate).
  4. Re-run this script with: SKIP_DEPLOY=1 ./20-create-appinsights.sh
NOTE
