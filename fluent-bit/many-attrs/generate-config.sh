#!/usr/bin/env bash

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NUM_ATTRS="${NUM_ATTRS:-400}"
LAST_INDEX=$((NUM_ATTRS - 1))

generate_dummy_json() {
  echo -n '{'
  echo -n "\"SeverityText\": \"Info\", \"Body\": \"hello from ${NUM_ATTRS} attributes test\""
  for i in $(seq 0 "$LAST_INDEX"); do
    name=$(printf "string%04d" "$i")
    value=$(printf "vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv_%04d" "$i")
    echo -n ", \"$name\": \"$value\""
  done
  echo -n '}'
}

DUMMY_JSON=$(generate_dummy_json)

cat > "$SCRIPT_DIR/fluent-bit.yaml" <<EOF
service:
  flush: 5

pipeline:
  inputs:
    - name: dummy
      dummy: '$DUMMY_JSON'

  outputs:
    - name: azure_logs_ingestion
      match: '*'
      table_name: ManyAttributesFluentBitLogs
      dce_url: \${DCE_URL}
      dcr_id: \${DCR_ID}
      tenant_id: \${TENANT_ID}
      client_id: \${CLIENT_ID}
      client_secret: \${CLIENT_SECRET}
      time_key: TimeGenerated
      time_generated: true
EOF

echo "Generated $SCRIPT_DIR/fluent-bit.yaml (NUM_ATTRS=$NUM_ATTRS)"
