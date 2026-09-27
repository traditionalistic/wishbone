#!/bin/sh
# Print a free TCP port for this workspace: a deterministic base derived from the
# workspace name (stable across restarts), then scan up past anything in use.
set -eu

name="${SUPERSET_WORKSPACE_NAME:-$(basename "$PWD")}"
port=$(( 8080 + $(printf '%s' "$name" | cksum | cut -d' ' -f1) % 400 ))
while lsof -iTCP:"$port" -sTCP:LISTEN -t >/dev/null 2>&1; do
	port=$(( port + 1 ))
done
echo "$port"
