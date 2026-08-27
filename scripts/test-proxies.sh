#!/bin/sh
set -eu
command -v docker >/dev/null || { echo "docker is required for proxy smoke tests" >&2; exit 1; }
root=$(mktemp -d "${TMPDIR:-/tmp}/trestle-proxies.XXXXXX")
caddy_id=
nginx_id=
server_pid=
cleanup() {
  [ -z "$caddy_id" ] || docker rm -f "$caddy_id" >/dev/null 2>&1 || true
  [ -z "$nginx_id" ] || docker rm -f "$nginx_id" >/dev/null 2>&1 || true
  [ -z "$server_pid" ] || kill "$server_pid" >/dev/null 2>&1 || true
  rm -rf "$root"
}
trap cleanup EXIT INT TERM
go build -o "$root/trestle" ./cmd/trestle
TRESTLE_LISTEN=0.0.0.0:18090 TRESTLE_DATA_DIR="$root/data" TRESTLE_TRUSTED_PROXIES=172.16.0.0/12 "$root/trestle" >"$root/trestle.log" 2>&1 &
server_pid=$!
for _ in $(seq 1 40); do curl -fsS http://127.0.0.1:18090/system/health >/dev/null && break; sleep .25; done

caddy_id=$(docker run -d --add-host host.docker.internal:host-gateway -p 18443:443 -v "$PWD/deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2)
for _ in $(seq 1 80); do curl -kfsS https://127.0.0.1:18443/system/health >/dev/null && break; sleep .25; done
curl -kfsS https://127.0.0.1:18443/system/version | grep -q '"version"'
docker rm -f "$caddy_id" >/dev/null
caddy_id=

mkdir -p "$root/certs"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=localhost -keyout "$root/certs/key.pem" -out "$root/certs/cert.pem" >/dev/null 2>&1
nginx_id=$(docker run -d --add-host host.docker.internal:host-gateway -p 18444:443 -v "$PWD/deploy/nginx/nginx.conf:/etc/nginx/nginx.conf:ro" -v "$root/certs:/etc/nginx/certs:ro" nginx:alpine)
for _ in $(seq 1 80); do curl -kfsS https://127.0.0.1:18444/system/health >/dev/null && break; sleep .25; done
curl -kfsS https://127.0.0.1:18444/system/version | grep -q '"version"'
