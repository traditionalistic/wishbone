#!/usr/bin/env bash
# Deploys the Wishbone server to a VPS next to whatever already runs there:
#
#     deploy/push.sh [root@host] [domain]
#
# Builds the admin and a static linux/amd64 server binary here, copies them with the contract to /opt/wishbone,
# runs docker compose (postgres + server on 127.0.0.1:8090) and puts an nginx site with a Let's Encrypt certificate
# in front. Rerun to update: .env (passwords) and the database survive. Screens are seeded only when missing —
# change a live screen through the admin.
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
cp -R "$ROOT/contract" "$STAGE/contract"
cp -R "$ROOT/admin/dist" "$STAGE/admin"

echo "» upload to $HOST:$REMOTE"
"${SSH[@]}" "mkdir -p $REMOTE && rm -rf $REMOTE/bin $REMOTE/contract $REMOTE/admin"
# COPYFILE_DISABLE: macOS tar would add AppleDouble ._* files, which the contract loader reads as JSON
COPYFILE_DISABLE=1 tar -C "$STAGE" -czf - . | "${SSH[@]}" "tar -xzf - -C $REMOTE"

echo "» compose, nginx, certificate"
"${SSH[@]}" "REMOTE=$REMOTE DOMAIN=$DOMAIN bash -s" <<'REMOTE_SCRIPT'
set -euo pipefail
cd "$REMOTE"
if [ ! -f .env ]; then
    umask 077
    printf 'POSTGRES_PASSWORD=%s\nWISHBONE_ADMIN_PASSWORD=%s\n' "$(openssl rand -hex 24)" "$(openssl rand -hex 12)" > .env
    echo "  new .env with generated passwords"
fi
docker compose up -d --build --remove-orphans
docker image prune -f >/dev/null

site=/etc/nginx/sites-available/wishbone
if [ ! -f "$site" ]; then
    cat > "$site" <<NGINX
server {
    listen 80;
    server_name $DOMAIN;
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
    ln -sf "$site" /etc/nginx/sites-enabled/wishbone
    nginx -t
    systemctl reload nginx
fi
if [ ! -d "/etc/letsencrypt/live/$DOMAIN" ]; then
    certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos --register-unsafely-without-email --redirect
fi
nginx -t 2>/dev/null && systemctl reload nginx

for i in $(seq 1 30); do
    curl -fsS "http://127.0.0.1:8090/v1/theme?brand=korpu" >/dev/null 2>&1 && break
    sleep 1
done
curl -fsS -o /dev/null -w "  server answers %{http_code}\n" "http://127.0.0.1:8090/v1/theme?brand=korpu"
REMOTE_SCRIPT

curl -fsS -o /dev/null -w "» https://$DOMAIN/v1/screens/host.hello → %{http_code}\n" "https://$DOMAIN/v1/screens/host.hello"
echo "Admin: https://$DOMAIN/admin/ — password: ssh $HOST 'grep ADMIN $REMOTE/.env'"
