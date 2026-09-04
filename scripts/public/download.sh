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
#   - more than one exact checksum entry matches;
#   - the entry is malformed;
#   - the archive bytes do not match its checksum;
#   - neither sha256sum nor shasum -a 256 is available.
#
# The exact entry is found by STRUCTURAL parsing, never by interpolating the
# archive filename into a regular expression. Every line is split on its first
# space; the entry is valid only when the hash field is exactly 64 lowercase
# hexadecimal characters, the separator is exactly two spaces, and the
# remaining text is byte-for-byte the expected bare filename with nothing
# after it. Dots and other regex metacharacters in version names can therefore
# never act as wildcards.
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

  _found=0
  _expected=
  while IFS= read -r _line; do
    _hash=${_line%% *}
    if printf '%s\n' "$_hash" | grep -E '^[0-9a-f]{64}$' >/dev/null 2>&1; then
      _rest=${_line#"$_hash"}
      # _rest must be exactly two spaces followed by the literal bare filename
      # and nothing else; the quoted expansion compares literally, so no regex
      # metacharacter in _name can match more than itself.
      case "$_rest" in
        "  $_name") _found=$((_found + 1)); _expected=$_hash ;;
      esac
    fi
  done < "$_sums"

  if [ "$_found" -eq 0 ]; then
    if grep -qF "$_name" "$_sums" 2>/dev/null; then
      echo "malformed checksum entry for $_name in SHA256SUMS" >&2
    else
      echo "no exact checksum entry for $_name in SHA256SUMS" >&2
    fi
    return 1
  fi
  if [ "$_found" -gt 1 ]; then
    echo "multiple matching checksum entries for $_name in SHA256SUMS" >&2
    return 1
  fi

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

# Canonical download.sh: downloads a checksum-verified Trestle executable into
# the current directory (or an explicit --output path) and changes nothing else.
# It never modifies PATH, shell startup files, ~/.local/bin, /usr/local/bin,
# system services, configuration, Trestle data or any existing file (unless
# --force is explicit). It does not execute the downloaded binary. In a
# repository checkout this sources scripts/checksum.sh; the standalone public
# copy served by the website inlines that helper so
# `curl -fsSL https://trestle.cv/download.sh | sh` needs no local file.

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

release_api=${TRESTLE_RELEASE_API_URL:-https://api.github.com/repos/trestle-cv/trestle/releases/latest}
release_base=${TRESTLE_RELEASE_BASE:-https://github.com/trestle-cv/trestle/releases/download}
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