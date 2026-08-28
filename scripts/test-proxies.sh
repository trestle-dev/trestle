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

bounded_diagnostics() {
  name=$1
  url=$2
  echo "--- proxy diagnostics: $name failed at $url ---" >&2
  if ! curl -fsS http://127.0.0.1:18090/system/health >/dev/null 2>&1; then
    echo "upstream trestle unreachable; trestle log tail:" >&2
    tail -n 30 "$root/trestle.log" 2>/dev/null || true
  fi
  if [ "$name" = "caddy" ] && [ -n "$caddy_id" ]; then
    echo "caddy container status:" >&2
    docker ps -a --filter "id=$caddy_id" --format '{{.ID}} {{.Status}}' 2>&1 || true
    echo "caddy container log tail:" >&2
    docker logs --tail 40 "$caddy_id" 2>&1 || true
  fi
  if [ "$name" = "nginx" ] && [ -n "$nginx_id" ]; then
    echo "nginx container status:" >&2
    docker ps -a --filter "id=$nginx_id" --format '{{.ID}} {{.Status}}' 2>&1 || true
    echo "nginx container log tail:" >&2
    docker logs --tail 40 "$nginx_id" 2>&1 || true
  fi
  echo "--- end proxy diagnostics ---" >&2
}

probe_proxy() {
  name=$1
  resolve=$2
  health_url=$3
  version_url=$4
  n=0
  while [ "$n" -lt 80 ]; do
    if curl --resolve "$resolve" -kfsS "$health_url" >/dev/null 2>&1; then
      break
    fi
    n=$((n + 1))
    sleep .25
  done
  if [ "$n" -ge 80 ]; then
    echo "PROXY SMOKE FAIL: $name never became ready at $health_url" >&2
    bounded_diagnostics "$name" "$health_url"
    return 1
  fi
  if ! curl --resolve "$resolve" -kfsS "$version_url" | grep -q '"version"'; then
    echo "PROXY SMOKE FAIL: $name version check failed at $version_url" >&2
    bounded_diagnostics "$name" "$version_url"
    return 1
  fi
  return 0
}

run_caddy() {
  caddy_id=$(docker run -d --add-host host.docker.internal:host-gateway -p 18443:443 -v "$PWD/deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2)
  if probe_proxy caddy "localhost:18443:127.0.0.1" "https://localhost:18443/system/health" "https://localhost:18443/system/version"; then
    docker rm -f "$caddy_id" >/dev/null 2>&1 || true
    caddy_id=
    return 0
  fi
  return 1
}

run_nginx() {
  mkdir -p "$root/certs"
  openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=localhost -keyout "$root/certs/key.pem" -out "$root/certs/cert.pem" >/dev/null 2>&1
  nginx_id=$(docker run -d --add-host host.docker.internal:host-gateway -p 18444:443 -v "$PWD/deploy/nginx/nginx.conf:/etc/nginx/nginx.conf:ro" -v "$root/certs:/etc/nginx/certs:ro" nginx:alpine)
  if probe_proxy nginx "localhost:18444:127.0.0.1" "https://localhost:18444/system/health" "https://localhost:18444/system/version"; then
    docker rm -f "$nginx_id" >/dev/null 2>&1 || true
    nginx_id=
    return 0
  fi
  return 1
}

go build -o "$root/trestle" ./cmd/trestle
TRESTLE_LISTEN=0.0.0.0:18090 TRESTLE_DATA_DIR="$root/data" TRESTLE_TRUSTED_PROXIES=172.16.0.0/12 "$root/trestle" >"$root/trestle.log" 2>&1 &
server_pid=$!
n=0
while [ "$n" -lt 40 ]; do
  if curl -fsS http://127.0.0.1:18090/system/health >/dev/null 2>&1; then
    break
  fi
  n=$((n + 1))
  sleep .25
done
if [ "$n" -ge 40 ]; then
  echo "PROXY SMOKE FAIL: upstream trestle never became ready on 18090" >&2
  tail -n 30 "$root/trestle.log" >&2 || true
  exit 1
fi

caddy_pass=0
if run_caddy; then
  caddy_pass=1
fi
nginx_pass=0
if run_nginx; then
  nginx_pass=1
fi

if [ "$caddy_pass" -ne 1 ] || [ "$nginx_pass" -ne 1 ]; then
  echo "PROXY SMOKE FAIL: caddy=$caddy_pass nginx=$nginx_pass" >&2
  exit 1
fi
echo "proxy smoke passed: caddy and nginx each proxied TLS to the trestle upstream"
