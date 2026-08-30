#!/bin/sh
set -eu

# Canonical download.sh: downloads a checksum-verified Trestle executable into
# the current directory (or an explicit --output path) and changes nothing else.
# It never modifies PATH, shell startup files, ~/.local/bin, /usr/local/bin,
# system services, configuration, Trestle data or any existing file (unless
# --force is explicit). It does not execute the downloaded binary. In a
# repository checkout this sources scripts/checksum.sh; the standalone public
# copy served by the website inlines that helper so
# `curl -fsSL https://trestle.cv/download.sh | sh` needs no local file.
root_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
. "$root_dir/scripts/checksum.sh"

version=${TRESTLE_VERSION:-latest}
force=false
output=trestle
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) shift; version=${1:?--version requires a value} ;;
    --force) force=true ;;
    --output) shift; output=${1:?--output requires a value} ;;
    --help) echo "usage: download.sh [--version VERSION] [--output PATH] [--force]"; exit 0 ;;
    --system) echo "download.sh is portable and never installs system-wide; use install.sh --system" >&2; exit 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done

case $(uname -s) in Linux) os=linux ;; Darwin) os=darwin ;; *) echo "use the Windows release archive on this platform" >&2; exit 1 ;; esac
case $(uname -m) in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo "unsupported architecture" >&2; exit 1 ;; esac

release_api=${TRESTLE_RELEASE_API_URL:-https://api.github.com/repos/trestle-dev/trestle/releases/latest}
release_base=${TRESTLE_RELEASE_BASE:-https://github.com/trestle-dev/trestle/releases/download}
if [ "$version" = latest ]; then
  version=$(curl -fsSL "$release_api" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
  [ -n "$version" ] || { echo "could not resolve the latest release" >&2; exit 1; }
fi

case "$output" in */*) dest_dir=$(dirname "$output") ;; *) dest_dir=. ;; esac
target=$output
[ -d "$dest_dir" ] || { echo "output directory does not exist: $dest_dir" >&2; exit 1; }
[ -w "$dest_dir" ] || { echo "output directory is not writable: $dest_dir" >&2; exit 1; }

if [ -d "$target" ]; then
  echo "refusing to overwrite the directory $target" >&2
  exit 1
fi
if [ -e "$target" ] || [ -L "$target" ]; then
  if $force; then
    echo "overwriting $target"
  else
    echo "refusing to overwrite existing $target; pass --force to replace it" >&2
    exit 1
  fi
fi

archive="trestle_${version#v}_${os}_${arch}.tar.gz"
work=$(mktemp -d "$dest_dir/.trestle-download.XXXXXX")
# Resolve to an absolute path: tar resolves a relative archive path relative to
# the -C directory, which would double the path.
work=$(CDPATH= cd -- "$work" && pwd)
trap 'rm -rf "$work"' EXIT INT TERM
curl -fsSL "${release_base}/${version}/${archive}" -o "$work/$archive"
curl -fsSL "${release_base}/${version}/SHA256SUMS" -o "$work/SHA256SUMS" || { echo "could not fetch SHA256SUMS; refusing an unverified archive" >&2; exit 1; }
verify_archive "$work/$archive" "$work/SHA256SUMS"
tar -xzf "$work/$archive" -C "$work"
binary="$work/trestle_${version#v}_${os}_${arch}/trestle"
[ -f "$binary" ] && [ ! -L "$binary" ] || { echo "release archive did not contain $archive's expected executable" >&2; exit 1; }
chmod 0755 "$binary"
mv "$binary" "$target"
echo "version: $version"
echo "platform: $os/$arch"
echo "downloaded: $target"
case "$output" in */*) display=$output ;; *) display="./$output" ;; esac
echo "Run $display"