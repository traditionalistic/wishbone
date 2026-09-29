#!/usr/bin/env bash
# Deploys the Wishbone server to a VPS next to whatever already runs there:
#
#     deploy/push.sh [root@host] [domain]
#
# Builds the admin and a static linux/amd64 server binary here, copies them with the contract to /opt/wishbone,
# runs docker compose (postgres + server on 127.0.0.1:8090) and puts an nginx site with a Let's Encrypt certificate
# in front. Rerun to update or repair: .env (passwords) and the database survive, the nginx site and the certificate
# install are redone every time. Screens are seeded only when missing — change a live screen through the admin.
#
# Needs here: node/npm, go, ssh with key access. On the VPS: docker compose v2, nginx (Debian sites-available
# layout), certbot with the nginx plugin, openssl, curl.
set -euo pipefail

HOST="${1:-root@217.198.9.139}"
DOMAIN="${2:-wishbone.217-198-9-139.sslip.io}"   # sslip.io resolves to the IP inside the name: no DNS record needed
REMOTE=/opt/wishbone
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STAGE="$(mktemp -d)"
CTL="/tmp/wishbone-ssh-$$"
trap 'rm -rf "$STAGE"; ssh -o ControlPath="$CTL" -O exit "$HOST" >/dev/null 2>&1 || true' EXIT
SSH=(ssh -o BatchMode=yes -o ControlMaster=auto -o "ControlPath=$CTL" -o ControlPersist=5m "$HOST")

echo "» admin"
(cd "$ROOT/admin" && npm ci --silent && npm run build --silent)
echo "» server binary"
(cd "$ROOT/backend" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$STAGE/bin/wishbone-server" ./cmd/server)
cp "$ROOT/deploy/Dockerfile" "$ROOT/deploy/docker-compose.yml" "$STAGE/"
[ -f /etc/ssl/cert.pem ] || { echo "missing /etc/ssl/cert.pem (macOS CA bundle)" >&2; exit 1; }
cp /etc/ssl/cert.pem "$STAGE/ca-certificates.crt"
cp -R "$ROOT/contract" "$STAGE/contract"
cp -R "$ROOT/admin/dist" "$STAGE/admin"
chmod 755 "$STAGE"

echo "» upload to $HOST:$REMOTE"
"${SSH[@]}" "mkdir -p $REMOTE && rm -rf $REMOTE/bin $REMOTE/contract $REMOTE/admin"
# COPYFILE_DISABLE / --no-xattrs: macOS tar would add AppleDouble ._* files, which the contract loader reads as JSON;
# --no-same-owner: files belong to root on the VPS, not to this Mac's uid
COPYFILE_DISABLE=1 tar --no-xattrs -C "$STAGE" -czf - . | "${SSH[@]}" "tar --no-same-owner -xzf - -C $REMOTE"

echo "» compose, nginx, certificate"
"${SSH[@]}" "REMOTE=$(printf %q "$REMOTE") DOMAIN=$(printf %q "$DOMAIN") bash -s" <<'REMOTE_SCRIPT'
set -euo pipefail
cd "$REMOTE"
chown root:root . && chmod 755 .
if [ ! -f .env ]; then
    pg="$(openssl rand -hex 24)"
    admin="$(openssl rand -hex 12)"
    [ ${#pg} -eq 48 ] && [ ${#admin} -eq 24 ] || { echo "openssl produced no passwords" >&2; exit 1; }
    (umask 077 && printf 'POSTGRES_PASSWORD=%s\nWISHBONE_ADMIN_PASSWORD=%s\n' "$pg" "$admin" > .env)
    echo "  new .env with generated passwords"
fi
if ! grep -q '^WISHBONE_GRAPHQL_URL=' .env; then
    (umask 077 && printf 'WISHBONE_GRAPHQL_URL=%s\nWISHBONE_GRAPHQL_LOGIN=%s\n' \
        "https://stage.korputeam.ru/graphql" "79956791664:133767" >> .env)
    echo "  .env: added upstream GraphQL vars"
fi
docker compose up -d --build --remove-orphans
docker image prune -f >/dev/null

# Rewritten on every run (certbot then re-adds its TLS lines), so template or domain changes always land.
cat > /etc/nginx/sites-available/wishbone <<NGINX
server {
    listen 80;
    server_name $DOMAIN;
    # the admin WebSocket authenticates with ?token=<password>: keep request lines out of the shared access log
    access_log off;
    client_max_body_size 4m;
    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        # preview WebSockets (admin ↔ devices) stay open while someone edits
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$http_connection;
        proxy_read_timeout 1h;
    }
}
NGINX
ln -sf /etc/nginx/sites-available/wishbone /etc/nginx/sites-enabled/wishbone
nginx -t
systemctl reload nginx
# idempotent: keeps a valid certificate and (re)installs it with the http → https redirect
certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos --register-unsafely-without-email --keep-until-expiring --redirect
nginx -t
systemctl reload nginx

for i in $(seq 1 30); do
    curl -fsS "http://127.0.0.1:8090/v1/theme?brand=korpu" >/dev/null 2>&1 && break
    sleep 1
done
curl -fsS -o /dev/null -w "  server answers %{http_code}\n" "http://127.0.0.1:8090/v1/theme?brand=korpu"
REMOTE_SCRIPT

curl -fsS -o /dev/null -w "» https://$DOMAIN/v1/screens/host.hello → %{http_code}\n" "https://$DOMAIN/v1/screens/host.hello"
echo "Admin: https://$DOMAIN/admin/ — password: ssh $HOST 'grep ADMIN $REMOTE/.env'"
