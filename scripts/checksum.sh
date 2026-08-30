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