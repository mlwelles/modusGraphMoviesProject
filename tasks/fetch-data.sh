#!/usr/bin/env bash
set -euo pipefail

DATA_DIR="${DATA_DIR:-data}"

mkdir -p "$DATA_DIR"

if [ -s "$DATA_DIR/1million.rdf.gz" ]; then
  echo "RDF data already present — skipping download."
else
  echo "Downloading 1million.rdf.gz …"
  curl -L -o "$DATA_DIR/1million.rdf.gz" \
    https://raw.githubusercontent.com/dgraph-io/tour/master/resources/1million.rdf.gz
fi

if [ -s "$DATA_DIR/1million.schema" ]; then
  echo "RDF schema already present — skipping download."
else
  echo "Downloading 1million.schema …"
  curl -L -o "$DATA_DIR/1million.schema" \
    https://raw.githubusercontent.com/dgraph-io/tour/master/resources/1million.schema
fi
