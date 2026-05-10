#!/usr/bin/env bash
# Shared variables sourced by the other infra scripts.
# Override anything by exporting before invoking a script, e.g.:
#   LOCATION=eastus2 ./10-create-cluster.sh

: "${SUBSCRIPTION_ID:=21f320bb-0abe-407e-aba8-480d084c058c}"
: "${LOCATION:=centralus}"
: "${BASE_NAME:=aks-otel-zap}"

: "${RESOURCE_GROUP:=${BASE_NAME}-rg}"
: "${AKS_NAME:=${BASE_NAME}-aks}"
: "${ACR_NAME:=${BASE_NAME//-/}acr}"        # ACR names must be alphanumeric, 5-50 chars
: "${LAW_NAME:=${BASE_NAME}-law}"           # Log Analytics workspace for Container Insights
: "${AMW_NAME:=${BASE_NAME}-amw}"           # Azure Monitor workspace (managed Prometheus + AppInsights metrics)
: "${AI_LAW_NAME:=${BASE_NAME}-ai-law}"     # Log Analytics workspace dedicated to Application Insights (must differ from LAW_NAME)
: "${APPINSIGHTS_NAME:=aks-otel-zap-app-insights-with-OTLP-support-via-portal}"
: "${APP_NAMESPACE:=aks-otel-zap}"

export SUBSCRIPTION_ID LOCATION BASE_NAME RESOURCE_GROUP AKS_NAME ACR_NAME \
       LAW_NAME AMW_NAME AI_LAW_NAME APPINSIGHTS_NAME APP_NAMESPACE
