#!/bin/sh
set -eu

system=false
version=${TRESTLE_VERSION:-latest}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --system) system=true ;;
    --version) shift; version=${1:?--version requires a value} ;;
    --help) echo "usage: install.sh [--system] [--version VERSION]"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done

if $system; then
  [ "$(id -u)" -eq 0 ] || { echo "--system requires root (use sudo)" >&2; exit 1; }
  install_dir=/usr/local/bin
else
  [ "$(id -u)" -ne 0 ] || { echo "refusing a per-user install as root; use --system" >&2; exit 1; }
  install_dir=${TRESTLE_INSTALL_DIR:-"$HOME/.local/bin"}
fi

case $(uname -s) in Linux) os=linux ;; Darwin) os=darwin ;; *) echo "use the Windows release archive on this platform" >&2; exit 1 ;; esac
case $(uname -m) in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "unsupported architecture" >&2; exit 1 ;; esac

if [ "$version" = latest ]; then
  version=$(curl -fsSL https://api.github.com/repos/trestle-dev/trestle/releases/latest | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
  [ -n "$version" ] || { echo "could not resolve the latest release" >&2; exit 1; }
fi
release_base=${TRESTLE_RELEASE_BASE:-https://github.com/trestle-dev/trestle/releases/download}
url="${release_base}/${version}/trestle_${version#v}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/trestle-install.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM
curl -fsSL "$url" -o "$tmp/trestle.tar.gz"
tar -xzf "$tmp/trestle.tar.gz" -C "$tmp"
binary=$(find "$tmp" -type f -name trestle -perm -u+x | head -1)
[ -n "$binary" ] || { echo "release archive did not contain trestle" >&2; exit 1; }
mkdir -p "$install_dir"
install -m 0755 "$binary" "$install_dir/trestle"
echo "installed trestle to $install_dir/trestle"
case ":$PATH:" in *":$install_dir:"*) ;; *) echo "add $install_dir to PATH" ;; esac
