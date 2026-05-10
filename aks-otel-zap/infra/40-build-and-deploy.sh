#!/usr/bin/env bash
# Build the Go service container image, push it to the ACR attached to the
# AKS cluster, and render k8s/deployment.yaml with the resolved ACR login
# server. Re-runnable.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=env.sh
source "$SCRIPT_DIR/env.sh"

APP_DIR="$(cd "$SCRIPT_DIR/../app" && pwd)"
K8S_DIR="$(cd "$SCRIPT_DIR/../k8s" && pwd)"

: "${IMAGE_TAG:=$(date +%Y%m%d%H%M%S)}"

echo "==> Resolving ACR login server for $ACR_NAME"
ACR_LOGIN_SERVER=$(az acr show -n "$ACR_NAME" -g "$RESOURCE_GROUP" --query loginServer -o tsv)
IMAGE_REF="$ACR_LOGIN_SERVER/aks-otel-zap:$IMAGE_TAG"

echo "==> Building and pushing $IMAGE_REF via 'az acr build' (no local Docker needed)"
az acr build \
  --registry "$ACR_NAME" \
  --image "aks-otel-zap:$IMAGE_TAG" \
  --image "aks-otel-zap:latest" \
  --file "$APP_DIR/Dockerfile" \
  "$APP_DIR"

echo "==> Rendering deployment.yaml with image $IMAGE_REF"
sed "s|ACR_LOGIN_SERVER/aks-otel-zap:latest|$IMAGE_REF|g" "$K8S_DIR/deployment.yaml" \
  > "$K8S_DIR/deployment.rendered.yaml"

echo "==> Applying rendered deployment"
kubectl apply -f "$K8S_DIR/deployment.rendered.yaml"

echo "==> Waiting for rollout..."
kubectl rollout status deployment/aks-otel-zap -n "$APP_NAMESPACE" --timeout=180s

cat <<NOTE

Image: $IMAGE_REF
Pods:
$(kubectl get pods -n "$APP_NAMESPACE" -o wide 2>&1)
NOTE
