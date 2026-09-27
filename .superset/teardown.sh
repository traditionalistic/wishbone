#!/bin/sh
# Stop whatever run.sh left listening in this workspace.
set -eu

if [ -f .superset/.port ]; then
	port=$(cat .superset/.port)
	pids=$(lsof -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null || true)
	[ -n "$pids" ] && kill $pids 2>/dev/null || true
	rm -f .superset/.port
fi

exit 0
