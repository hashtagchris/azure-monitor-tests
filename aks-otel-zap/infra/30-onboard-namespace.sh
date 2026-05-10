#!/usr/bin/env bash
# Apply the Instrumentation CR to enable AKS Application Monitoring
# autoconfiguration (env-var injection) for the aks-otel-zap namespace, then
# restart the deployment so the addon's mutating webhook gets a chance to
# inject OTEL_EXPORTER_OTLP_* env vars.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=env.sh
source "$SCRIPT_DIR/env.sh"

if [[ ! -f "$SCRIPT_DIR/appinsights.json" ]]; then
  echo "ERROR: $SCRIPT_DIR/appinsights.json not found. Run 20-create-appinsights.sh first." >&2
  exit 1
fi

CONN_STRING=$(jq -r '.properties.ConnectionString' "$SCRIPT_DIR/appinsights.json")
if [[ -z "$CONN_STRING" || "$CONN_STRING" == "null" ]]; then
  echo "ERROR: AppInsights connection string missing from appinsights.json." >&2
  exit 1
fi

K8S_DIR="$(cd "$SCRIPT_DIR/../k8s" && pwd)"

echo "==> Applying namespace + Instrumentation CR"
kubectl apply -f "$K8S_DIR/namespace.yaml"
APPLICATIONINSIGHTS_CONNECTION_STRING="$CONN_STRING" envsubst \
  < "$K8S_DIR/instrumentation.yaml" | kubectl apply -f -

echo "==> Current Instrumentation CR:"
kubectl get instrumentation -n "$APP_NAMESPACE" -o yaml | grep -E '^(  name:|    applicationInsightsConnectionString:|    autoInstrumentationPlatforms:)' || true

echo "==> Restarting deployment $APP_NAMESPACE/aks-otel-zap (no-op if not yet deployed)"
kubectl rollout restart deployment/aks-otel-zap -n "$APP_NAMESPACE" 2>/dev/null || \
  echo "    (deployment not yet present; apply k8s/deployment.yaml first)"

cat <<NOTE

Once the Go service pod is running, verify that the Application Monitoring
addon injected OTLP env vars with:
  kubectl exec -n $APP_NAMESPACE deploy/aks-otel-zap -- env | grep OTEL_
NOTE
