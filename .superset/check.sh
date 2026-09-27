#!/bin/sh
# Self-check for the workspace lifecycle scripts. Run from the repo root.
set -eu
cd "$(dirname "$0")/.."
fail() { echo "FAIL: $1" >&2; exit 1; }

# setup must succeed on a placeholder checkout and copy untracked root files
root=$(mktemp -d); ws=$(mktemp -d); trap 'rm -rf "$root" "$ws"' EXIT
printf 'SECRET=1\n' > "$root/.env"
cp -R .superset "$ws/"
(cd "$ws" && SUPERSET_ROOT_PATH="$root" sh .superset/setup.sh >/dev/null) || fail "setup.sh exited non-zero"
[ "$(cat "$ws/.env")" = "SECRET=1" ] || fail "setup.sh did not copy .env from \$SUPERSET_ROOT_PATH"

# port must be deterministic per workspace, and differ between workspaces
a=$(SUPERSET_WORKSPACE_NAME=feat-a ./.superset/free-port.sh)
[ "$a" = "$(SUPERSET_WORKSPACE_NAME=feat-a ./.superset/free-port.sh)" ] || fail "port not deterministic"
[ "$a" != "$(SUPERSET_WORKSPACE_NAME=feat-b ./.superset/free-port.sh)" ] || fail "workspaces collide on one port"

# an occupied port must be skipped, and teardown must free it again
hold_port() {  # wrapper child turns the kill into a plain exit, so no job notice leaks
	sh -c "python3 -c \"import socket,time
s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
s.bind(('127.0.0.1',$1)); s.listen(1); time.sleep(20)\"; :" >/dev/null 2>&1
}
hold_port "$a" &
held=$!
sleep 1.5
[ "$(SUPERSET_WORKSPACE_NAME=feat-a ./.superset/free-port.sh)" != "$a" ] || fail "busy port not skipped"
echo "$a" > .superset/.port
sh .superset/teardown.sh || fail "teardown.sh exited non-zero"
sleep 0.5
[ -z "$(lsof -iTCP:"$a" -sTCP:LISTEN -t 2>/dev/null || true)" ] || fail "teardown left a listener on $a"
[ ! -f .superset/.port ] || fail "teardown left .superset/.port behind"
kill $held 2>/dev/null || true

sh .superset/teardown.sh || fail "teardown.sh not idempotent"
echo "OK: setup, free-port, teardown all behave"
