#!/bin/sh
# Compiles playground fixtures (Fixtures/*.json + contract/screens/*.json) into bundled screen answers.
set -e
cd "$(dirname "$0")"
out=../Resources/Fixtures
mkdir -p "$out"
for src in *.json ../../../contract/screens/*.json; do
  id=$(basename "$src" .json)
  doc=$(cd ../../../backend && go run ./cmd/wbc compile -contract ../contract "$OLDPWD/$src")
  printf '{"etag":"fixture","revision":0,"data":{},"document":%s}\n' "$doc" > "$out/$id.json"
  echo "$out/$id.json"
done
