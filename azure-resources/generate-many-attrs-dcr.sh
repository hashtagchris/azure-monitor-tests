#!/usr/bin/env bash

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NUM_ATTRS="${NUM_ATTRS:-400}"
LAST_INDEX=$((NUM_ATTRS - 1))

generate_columns() {
  echo '['
  echo '          {"name": "TimeGenerated", "type": "datetime"},'
  echo '          {"name": "SeverityText", "type": "string"},'
  echo '          {"name": "Body", "type": "string"},'
  for i in $(seq 0 "$LAST_INDEX"); do
    name=$(printf "string%04d" "$i")
    if [ "$i" -eq "$LAST_INDEX" ]; then
      echo "          {\"name\": \"$name\", \"type\": \"string\"}"
    else
      echo "          {\"name\": \"$name\", \"type\": \"string\"},"
    fi
  done
  echo '        ]'
}

generate_bag_pack_args() {
  local args=""
  for i in $(seq 0 "$LAST_INDEX"); do
    name=$(printf "string%04d" "$i")
    if [ -n "$args" ]; then
      args="$args, "
    fi
    # Escape quotes for embedding inside a JSON string value
    args="$args\\\"$name\\\", $name"
  done
  echo "$args"
}

generate_dcr() {
  local workspace_resource_id="$1"
  local bag_pack_args
  bag_pack_args=$(generate_bag_pack_args)
  local transform_kql="source | project TimeGenerated, SeverityText, Body, Attributes = bag_pack($bag_pack_args)"

  cat <<EOF
{
  "properties": {
    "streamDeclarations": {
      "Custom-ManyAttributesFluentBitLogs": {
        "columns": $(generate_columns)
      }
    },
    "destinations": {
      "logAnalytics": [
        {
          "workspaceResourceId": "$workspace_resource_id",
          "name": "myworkspace"
        }
      ]
    },
    "dataFlows": [
      {
        "streams": ["Custom-ManyAttributesFluentBitLogs"],
        "destinations": ["myworkspace"],
        "transformKql": "$transform_kql",
        "outputStream": "Microsoft-OTel-Logs"
      }
    ]
  }
}
EOF
}

# Generate template version (with placeholders)
TEMPLATE_WORKSPACE="/subscriptions/\${SUBSCRIPTION_ID}/resourceGroups/\${RESOURCE_GROUP}/providers/Microsoft.OperationalInsights/workspaces/\${WORKSPACE_NAME}"
generate_dcr "$TEMPLATE_WORKSPACE" > "$SCRIPT_DIR/dcr-many-attrs.json.tmpl"
echo "Generated $SCRIPT_DIR/dcr-many-attrs.json.tmpl (NUM_ATTRS=$NUM_ATTRS)"
