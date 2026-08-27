#!/bin/sh
set -eu
system=false
version=${TRESTLE_VERSION:-latest}
dry_run=false
rollback=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --system) system=true ;;
    --version) shift; version=${1:?--version requires a value} ;;
    --dry-run) dry_run=true ;;
    --rollback) rollback=true ;;
    --help) echo "usage: update.sh [--system] [--version VERSION] [--dry-run] [--rollback]"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done
if $system; then
  [ "$(id -u)" -eq 0 ] || { echo "--system requires root (use sudo)" >&2; exit 1; }
  binary=/usr/local/bin/trestle
else
  [ "$(id -u)" -ne 0 ] || { echo "refusing a per-user update as root; use --system" >&2; exit 1; }
  binary=${TRESTLE_INSTALL_DIR:-"$HOME/.local/bin"}/trestle
fi
[ -f "$binary" ] || { echo "no Trestle installation at $binary" >&2; exit 1; }
[ ! -L "$binary" ] || { echo "refusing to replace symlinked installation: $binary" >&2; exit 1; }
previous="$binary.previous"
if $rollback; then
  [ -f "$previous" ] || { echo "no rollback binary at $previous" >&2; exit 1; }
  $dry_run && { echo "would restore $previous to $binary"; exit 0; }
  staging="$binary.rollback.$$"
  cp "$previous" "$staging"
  chmod 0755 "$staging"
  mv "$staging" "$binary"
  echo "restored previous Trestle binary"
  exit 0
fi
case $(uname -s) in Linux) os=linux ;; Darwin) os=darwin ;; *) echo "use the Windows release archive on this platform" >&2; exit 1 ;; esac
case $(uname -m) in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "unsupported architecture" >&2; exit 1 ;; esac
if [ "$version" = latest ]; then
  version=$(curl -fsSL https://api.github.com/repos/trestle-dev/trestle/releases/latest | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
  [ -n "$version" ] || { echo "could not resolve the latest release" >&2; exit 1; }
fi
current=$($binary version 2>/dev/null | head -1 || true)
echo "installed: ${current:-unknown}"
echo "target: $version"
$dry_run && { echo "would verify and atomically replace $binary; data and configuration are untouched"; exit 0; }
release_base=${TRESTLE_RELEASE_BASE:-https://github.com/trestle-dev/trestle/releases/download}
archive="trestle_${version#v}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/trestle-update.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM
curl -fsSL "${release_base}/${version}/${archive}" -o "$tmp/$archive"
curl -fsSL "${release_base}/${version}/SHA256SUMS" -o "$tmp/SHA256SUMS"
(cd "$tmp" && grep "  $archive\$" SHA256SUMS > selected.sha256 && [ -s selected.sha256 ] && sha256sum -c selected.sha256)
tar -xzf "$tmp/$archive" -C "$tmp"
candidate=$(find "$tmp" -type f -name trestle -perm -u+x | head -1)
[ -n "$candidate" ] || { echo "release archive did not contain trestle" >&2; exit 1; }
cp "$binary" "$previous.new"
chmod 0755 "$previous.new"
mv "$previous.new" "$previous"
install -m 0755 "$candidate" "$binary.new"
mv "$binary.new" "$binary"
echo "updated Trestle to $version; rollback with update.sh --rollback"
