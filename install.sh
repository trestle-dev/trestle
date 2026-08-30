#!/bin/sh
set -eu

# Canonical install.sh: installs the verified Trestle executable into
# ~/.local/bin (default) or /usr/local/bin (--system). Release archives and
# SHA256SUMS are downloaded and the archive is verified before anything is
# written. In a repository checkout this sources scripts/checksum.sh; the
# standalone public copy served by the website inlines that helper so
# `curl -fsSL https://trestle.cv/install.sh | sh` needs no local file.
root_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
. "$root_dir/scripts/checksum.sh"

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

release_api=${TRESTLE_RELEASE_API_URL:-https://api.github.com/repos/trestle-dev/trestle/releases/latest}
release_base=${TRESTLE_RELEASE_BASE:-https://github.com/trestle-dev/trestle/releases/download}
if [ "$version" = latest ]; then
  version=$(curl -fsSL "$release_api" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
  [ -n "$version" ] || { echo "could not resolve the latest release" >&2; exit 1; }
fi
archive="trestle_${version#v}_${os}_${arch}.tar.gz"
url="${release_base}/${version}/${archive}"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/trestle-install.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM
curl -fsSL "$url" -o "$tmp/$archive"
curl -fsSL "${release_base}/${version}/SHA256SUMS" -o "$tmp/SHA256SUMS" || { echo "could not fetch SHA256SUMS; refusing an unverified archive" >&2; exit 1; }
verify_archive "$tmp/$archive" "$tmp/SHA256SUMS"
tar -xzf "$tmp/$archive" -C "$tmp"
binary="$tmp/trestle_${version#v}_${os}_${arch}/trestle"
[ -f "$binary" ] && [ ! -L "$binary" ] || { echo "release archive did not contain $archive's expected executable" >&2; exit 1; }
mkdir -p "$install_dir"
install -m 0755 "$binary" "$install_dir/trestle"
echo "installed trestle $version ($os/$arch) to $install_dir/trestle"
case ":$PATH:" in *":$install_dir:"*) ;; *) echo "add $install_dir to PATH" ;; esac