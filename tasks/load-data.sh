#!/usr/bin/env bash
set -euo pipefail

CONTAINER="${DGRAPH_CONTAINER:-modus-movies-dgraph}"
DATA_DIR="${DATA_DIR:-data}"

if [ ! -f "$DATA_DIR/1million.rdf.gz" ] || [ ! -f "$DATA_DIR/1million.schema" ]; then
  echo "Error: data files not found in $DATA_DIR/. Run fetch-data first."
  exit 1
fi

echo "Copying data files into container $CONTAINER …"
docker cp "$DATA_DIR/1million.rdf.gz" "$CONTAINER:/tmp/1million.rdf.gz"
docker cp "$DATA_DIR/1million.schema" "$CONTAINER:/tmp/1million.schema"

echo "Loading data with dgraph live …"
docker exec "$CONTAINER" dgraph live \
  -f /tmp/1million.rdf.gz \
  -s /tmp/1million.schema
