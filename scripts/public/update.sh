# Shared release-archive checksum verification (canonical helper).
#
# This file is the single source of truth for portable SHA-256 verification of
# Trestle release archives. install.sh, download.sh and update.sh source it
# from a repository checkout; scripts/build-public-scripts.sh inlines it into
# the standalone public copies so `curl ... | sh` never depends on a local
# helper file.
#
# verify_archive fails closed on every deviation:
#   - SHA256SUMS is unavailable;
#   - the selected archive has no exact checksum entry;
#   - the entry is malformed;
#   - the archive bytes do not match its checksum;
#   - neither sha256sum nor shasum -a 256 is available.
#
# Tool selection is portable: sha256sum when present, otherwise shasum -a 256.
# TRESTLE_CHECKSUM_TOOL forces a choice (auto | sha256sum | shasum | none) for
# deterministic tests; "none" exercises the fail-closed no-tool path.

# verify_archive ARCHIVE SHA256SUMS_FILE
# Prints the verified hex digest on success and returns 0. ARCHIVE may be a
# path; the SHA256SUMS entry is matched by the bare archive filename.
# Parameter and intermediate names are _-prefixed so the function never
# overwrites a caller's variables (POSIX sh has no local scoping).
verify_archive() {
  _archive=$1
  _sums=$2
  _name=$(basename "$_archive")

  if [ ! -f "$_sums" ]; then
    echo "SHA256SUMS is unavailable; refusing an unverified download" >&2
    return 1
  fi

  # The exact checksum entry for this archive: 64 lowercase hex, two spaces,
  # then the archive filename and nothing else.
  _line=$(grep -E "^[0-9a-f]{64}  ${_name}$" "$_sums" || true)
  if [ -z "$_line" ]; then
    if grep -q "${_name}$" "$_sums" 2>/dev/null; then
      echo "malformed checksum entry for $_name in SHA256SUMS" >&2
    else
      echo "no exact checksum entry for $_name in SHA256SUMS" >&2
    fi
    return 1
  fi
  _expected=${_line%% *}
  case "$_expected" in
    [0-9a-f][0-9a-f]*) ;;
    *) echo "malformed checksum entry for $_name in SHA256SUMS" >&2; return 1 ;;
  esac

  _tool=${TRESTLE_CHECKSUM_TOOL:-auto}
  case "$_tool" in
    auto)
      if command -v sha256sum >/dev/null 2>&1; then
        _tool=sha256sum
      elif command -v shasum >/dev/null 2>&1; then
        _tool=shasum
      else
        echo "neither sha256sum nor shasum -a 256 is available" >&2
        return 1
      fi
      ;;
    sha256sum)
      command -v sha256sum >/dev/null 2>&1 || { echo "sha256sum is not available" >&2; return 1; }
      ;;
    shasum)
      command -v shasum >/dev/null 2>&1 || { echo "shasum is not available" >&2; return 1; }
      ;;
    none)
      echo "neither sha256sum nor shasum -a 256 is available" >&2
      return 1
      ;;
    *)
      echo "unknown TRESTLE_CHECKSUM_TOOL: $_tool" >&2
      return 1
      ;;
  esac

  if [ "$_tool" = sha256sum ]; then
    _actual=$(sha256sum "$_archive" 2>/dev/null | awk '{print $1}')
  else
    _actual=$(shasum -a 256 "$_archive" 2>/dev/null | awk '{print $1}')
  fi
  if [ -z "$_actual" ]; then
    echo "could not compute the checksum of $_archive" >&2
    return 1
  fi
  if [ "$_actual" != "$_expected" ]; then
    echo "checksum mismatch for $_name: expected $_expected, got $_actual" >&2
    return 1
  fi
  echo "verified $_name ($_expected)"
  return 0
}
#!/bin/sh
set -eu

# Canonical update.sh: verifies and atomically replaces the installed Trestle
# executable, retaining the previous binary for rollback. It never modifies
# instance data or configuration. In a repository checkout this sources
# scripts/checksum.sh; the standalone public copy served by the website inlines
# that helper so `curl -fsSL https://trestle.cv/update.sh | sh` needs no local
# file.

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
release_api=${TRESTLE_RELEASE_API_URL:-https://api.github.com/repos/trestle-dev/trestle/releases/latest}
release_base=${TRESTLE_RELEASE_BASE:-https://github.com/trestle-dev/trestle/releases/download}
if [ "$version" = latest ]; then
  version=$(curl -fsSL "$release_api" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
  [ -n "$version" ] || { echo "could not resolve the latest release" >&2; exit 1; }
fi
current=$($binary version 2>/dev/null | head -1 || true)
echo "installed: ${current:-unknown}"
echo "target: $version"
$dry_run && { echo "would verify and atomically replace $binary; data and configuration are untouched"; exit 0; }
archive="trestle_${version#v}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/trestle-update.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM
curl -fsSL "${release_base}/${version}/${archive}" -o "$tmp/$archive"
curl -fsSL "${release_base}/${version}/SHA256SUMS" -o "$tmp/SHA256SUMS" || { echo "could not fetch SHA256SUMS; refusing an unverified archive" >&2; exit 1; }
verify_archive "$tmp/$archive" "$tmp/SHA256SUMS"
tar -xzf "$tmp/$archive" -C "$tmp"
candidate="$tmp/trestle_${version#v}_${os}_${arch}/trestle"
[ -f "$candidate" ] && [ ! -L "$candidate" ] || { echo "release archive did not contain $archive's expected executable" >&2; exit 1; }
cp "$binary" "$previous.new"
chmod 0755 "$previous.new"
mv "$previous.new" "$previous"
install -m 0755 "$candidate" "$binary.new"
mv "$binary.new" "$binary"
echo "updated Trestle to $version; rollback with update.sh --rollback"