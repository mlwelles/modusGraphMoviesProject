#!/usr/bin/env bash
set -euo pipefail

DGRAPH_ALPHA="${DGRAPH_ALPHA:-http://localhost:8080}"

echo "Dropping all data from $DGRAPH_ALPHA …"
RESPONSE=$(curl -s -X POST "$DGRAPH_ALPHA/alter" -d '{"drop_all": true}')

if echo "$RESPONSE" | grep -q '"code":"Success"'; then
  echo "All data dropped successfully."
else
  echo "Error dropping data:"
  echo "$RESPONSE"
  exit 1
fi
