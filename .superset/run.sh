#!/bin/sh
# Launch the BDUI backend on a port that is free in this workspace.
set -eu

port=$(./.superset/free-port.sh)
echo "$port" > .superset/.port

if [ ! -f backend/go.mod ]; then
	echo "backend/ is still a placeholder — no dev server to run yet." >&2
	exit 1
fi

# always rebuild: a stale admin/dist would serve an old editor
if [ -f admin/package.json ]; then
	(cd admin && npm run build)
fi

cd backend
export PORT="$port"
echo "BDUI backend on http://localhost:$port"
if [ -d ./cmd/server ]; then
	exec go run ./cmd/server
else
	exec go run .
fi
