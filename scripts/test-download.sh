#!/bin/sh
# CP16 download.sh regression: deterministic tests against a local fake release
# tree (no network, no dependency on GitHub availability). Exercises every
# documented download.sh behavior, the standalone public copy the website
# serves, portable checksum verification, platform mapping, overwrite safety,
# atomic staging cleanup, and a packaged release run as ./trestle version.
#
# The protected system-binary path (default /usr/local/bin/trestle) is
# preserved by a before/after fingerprint contract rather than an absence
# assertion, so the suite passes on a machine where Trestle is legitimately
# installed and running. Override with TRESTLE_TEST_SYSTEM_BINARY to exercise
# the preservation helper against a disposable path.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
download="$root/scripts/public/download.sh"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/trestle-download-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

failures=0
fail() { echo "download test FAIL: $*" >&2; failures=$((failures + 1)); }

# --- Fake release tree ------------------------------------------------------
rel="$tmp/releases"
ver=v0.0.0-test
rel_dir="$rel/$ver"
mkdir -p "$rel_dir/stage"
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  stage="$rel_dir/stage/trestle_0.0.0-test_$p"
  mkdir -p "$stage"
  printf '#!/bin/sh\necho fake-%s\n' "$p" > "$stage/trestle"
  chmod +x "$stage/trestle"
  tar -C "$rel_dir/stage" -czf "$rel_dir/trestle_0.0.0-test_$p.tar.gz" "trestle_0.0.0-test_$p"
done
(cd "$rel_dir" && sha256sum trestle_0.0.0-test_*.tar.gz > SHA256SUMS)
printf '{"tag_name":"v0.0.0-test"}\n' > "$rel/latest.json"
release_base="file://$rel"

# --- Fake uname + shasum for deterministic platform/tool tests -------------
fakebin="$tmp/fakebin"
mkdir -p "$fakebin"
cat > "$fakebin/uname" <<'EOF'
#!/bin/sh
case "$1" in
  -s) echo "${TRESTLE_TEST_UNAME_S:-Linux}" ;;
  -m) echo "${TRESTLE_TEST_UNAME_M:-x86_64}" ;;
  *) exec /usr/bin/uname "$@" ;;
esac
EOF
cat > "$fakebin/shasum" <<'EOF'
#!/bin/sh
while [ "$#" -gt 0 ]; do case "$1" in -a) shift; shift ;; *) break ;; esac; done
exec sha256sum "$@"
EOF
chmod +x "$fakebin/uname" "$fakebin/shasum"

# --- Helpers -----------------------------------------------------------------
# new_dest sets $dest to a fresh scratch destination directory in the parent
# shell (no command substitution, so the counter persists).
dest_n=0
new_dest() {
  dest_n=$((dest_n + 1))
  dest="$tmp/dest-$dest_n"
  mkdir -p "$dest"
}

# Portable stat probes. GNU stat uses -c, BSD/macOS stat uses -f; the field
# selectors below produce equivalent octal-mode, epoch-second-mtime and
# uid:gid output on both.
if stat -c '%a' "$tmp" >/dev/null 2>&1; then
  stat_mode() { stat -c '%a' "$1"; }
  stat_mtime() { stat -c '%Y' "$1"; }
  stat_owner() { stat -c '%u:%g' "$1"; }
else
  stat_mode() { stat -f '%Lp' "$1"; }
  stat_mtime() { stat -f '%m' "$1"; }
  stat_owner() { stat -f '%u:%g' "$1"; }
fi

# snapshot PATH emits a stable single-line fingerprint proving non-mutation.
# A symlink is reported by its target without dereferencing and without a
# link-metadata claim (only target preservation is asserted); a regular file is
# hashed and sized; any other special file reports type/mode/mtime/owner only
# (never dereferenced or hashed unsafely). An absent path reports MISSING.
snapshot() {
  p=$1
  if [ -L "$p" ]; then
    printf 'symlink -> %s\n' "$(readlink "$p")"
  elif [ -e "$p" ]; then
    if [ -f "$p" ]; then
      if command -v sha256sum >/dev/null 2>&1; then
        h=$(sha256sum "$p" | awk '{print $1}')
      else
        h=$(shasum -a 256 "$p" | awk '{print $1}')
      fi
      printf 'regular sha256=%s size=%s mtime=%s mode=%s owner=%s\n' \
        "$h" "$(wc -c < "$p")" "$(stat_mtime "$p")" "$(stat_mode "$p")" "$(stat_owner "$p")"
    else
      printf 'special mtime=%s mode=%s owner=%s\n' \
        "$(stat_mtime "$p")" "$(stat_mode "$p")" "$(stat_owner "$p")"
    fi
  else
    printf 'MISSING\n'
  fi
}

# fp_field FIELD FINGERPRINT extracts one name=value component from a regular-file
# fingerprint, so negative controls assert a specific dimension (e.g. sha256,
# mode) rather than merely that two fingerprints differ.
fp_field() {
  printf '%s' "$2" | sed -n "s/.* $1=\([^ ]*\).*/\1/p"
}

# --- 1. Latest-version resolution (via the local release API) ---------------
new_dest
out=$( (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" TRESTLE_RELEASE_API_URL="file://$rel/latest.json" sh "$download") 2>&1 || true )
echo "$out" | grep -q "version: v0.0.0-test" || fail "latest resolution: got $out"
[ -x "$dest/trestle" ] || fail "latest resolution produced no executable"

# --- 2. Explicit version -----------------------------------------------------
new_dest
out=$( (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver") 2>&1 )
echo "$out" | grep -q "version: $ver" || fail "explicit version: $out"
[ -x "$dest/trestle" ] || fail "explicit version produced no executable"

# --- 3-6. Platform mapping (fake uname) --------------------------------------
for t in "Linux:x86_64:linux:amd64" "Linux:aarch64:linux:arm64" "Darwin:x86_64:darwin:amd64" "Darwin:arm64:darwin:arm64"; do
  s=${t%%:*}; rest=${t#*:}; m=${rest%%:*}; rest=${rest#*:}; o=${rest%%:*}; a=${rest#*:}
  new_dest
  out=$( (cd "$dest" && TRESTLE_TEST_UNAME_S="$s" TRESTLE_TEST_UNAME_M="$m" PATH="$fakebin:$PATH" TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver") 2>&1 )
  echo "$out" | grep -q "platform: $o/$a" || fail "$s/$m mapping: $out"
done

# --- 7. Checksum verification with sha256sum (default on Linux) --------------
new_dest
out=$( (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver") 2>&1 )
echo "$out" | grep -q "^verified trestle_0.0.0-test_linux_amd64.tar.gz " || fail "sha256sum verification: $out"

# --- 8. Checksum verification with shasum -a 256 -----------------------------
new_dest
out=$( (cd "$dest" && PATH="$fakebin:$PATH" TRESTLE_RELEASE_BASE="$release_base" TRESTLE_CHECKSUM_TOOL=shasum sh "$download" --version "$ver") 2>&1 )
echo "$out" | grep -q "^verified trestle_0.0.0-test_linux_amd64.tar.gz " || fail "shasum verification: $out"

# --- 9. Missing checksum file fails closed -----------------------------------
mkdir -p "$rel/nosums/$ver"
tar -C "$rel_dir/stage" -czf "$rel/nosums/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/nosums" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "missing SHA256SUMS did not fail closed"
fi

# --- 10. Missing exact archive entry fails closed ----------------------------
mkdir -p "$rel/noentry/$ver"
tar -C "$rel_dir/stage" -czf "$rel/noentry/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
printf '0000000000000000000000000000000000000000000000000000000000000000  other.tar.gz\n' > "$rel/noentry/$ver/SHA256SUMS"
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/noentry" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "missing exact checksum entry did not fail closed"
fi

# --- 11. Malformed checksum entry fails closed --------------------------------
mkdir -p "$rel/malformed/$ver"
tar -C "$rel_dir/stage" -czf "$rel/malformed/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
printf 'zzz  trestle_0.0.0-test_linux_amd64.tar.gz\n' > "$rel/malformed/$ver/SHA256SUMS"
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/malformed" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "malformed checksum entry did not fail closed"
fi

# --- 12. Corrupted archive fails closed ---------------------------------------
mkdir -p "$rel/corrupt/$ver"
tar -C "$rel_dir/stage" -czf "$rel/corrupt/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
(cd "$rel/corrupt/$ver" && sha256sum trestle_0.0.0-test_linux_amd64.tar.gz > SHA256SUMS)
python3 -c "p='$rel/corrupt/$ver/trestle_0.0.0-test_linux_amd64.tar.gz'; d=open(p,'rb').read(); open(p,'wb').write(d[:-1])"
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/corrupt" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "corrupted archive did not fail closed"
fi

# --- 13. Archive missing the expected binary fails closed ---------------------
mkdir -p "$rel/nobin/$ver"
mkdir -p "$rel/nobin/stage/trestle_0.0.0-test_linux_amd64"
printf '#!/bin/sh\necho not-the-binary\n' > "$rel/nobin/stage/trestle_0.0.0-test_linux_amd64/README"
tar -C "$rel/nobin/stage" -czf "$rel/nobin/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
(cd "$rel/nobin/$ver" && sha256sum trestle_0.0.0-test_linux_amd64.tar.gz > SHA256SUMS)
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/nobin" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "archive missing the expected executable did not fail closed"
fi

# --- 13b. Exact checksum entry matching is literal, not a regex ----------------
# The archive filename (trestle_0.0.0-test_linux_amd64.tar.gz) contains dots; a
# structural parser must reject near matches, prefixes, suffixes and duplicates.
real_hash=$(awk '{print $1}' "$rel_dir/SHA256SUMS" | head -1)
mkvariant() { # mkvariant NAME SHA256SUMS_CONTENT
  mkdir -p "$rel/$1/$ver"
  cp "$rel_dir/trestle_0.0.0-test_linux_amd64.tar.gz" "$rel/$1/$ver/"
  printf '%s\n' "$2" > "$rel/$1/$ver/SHA256SUMS"
}
expect_fail() { # expect_fail LABEL VARIANT
  new_dest
  if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/$2" sh "$download" --version "$ver" >/dev/null 2>&1); then
    fail "$1: near/duplicate/malformed entry did not fail closed"
  fi
}
mkvariant wildcards "$real_hash  trestle_0x0x0-test_linux_amd64XtarZgz"
expect_fail "dots in the version name acting as wildcards" wildcards
mkvariant near-match "$real_hash  trestle_0-0-0-test_linux_amd64-tar-gz"
expect_fail "regex near-match filename" near-match
mkvariant prefix "$real_hash  xtrestle_0.0.0-test_linux_amd64.tar.gz"
expect_fail "prefix variant" prefix
mkvariant suffix "$real_hash  trestle_0.0.0-test_linux_amd64.tar.gzx"
expect_fail "suffix variant" suffix
mkvariant duplicate "$real_hash  trestle_0.0.0-test_linux_amd64.tar.gz
$real_hash  trestle_0.0.0-test_linux_amd64.tar.gz"
expect_fail "duplicate exact entries" duplicate
mkvariant malformed "zzz  trestle_0.0.0-test_linux_amd64.tar.gz"
expect_fail "malformed exact-name entry" malformed

# --- 14. Existing destination file refusal ------------------------------------
new_dest
printf 'keep me\n' > "$dest/trestle"
if (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "existing destination file was not refused"
fi
[ "$(cat "$dest/trestle")" = "keep me" ] || fail "existing destination file was modified"

# --- 15. Existing destination directory refusal --------------------------------
new_dest
mkdir -p "$dest/trestle"
if (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "existing destination directory was not refused"
fi
[ -d "$dest/trestle" ] || fail "existing destination directory was removed"

# --- 16. Existing destination symlink refusal ---------------------------------
new_dest
ln -s /dev/null "$dest/trestle"
if (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "existing destination symlink was not refused"
fi
[ -L "$dest/trestle" ] || fail "existing destination symlink was replaced without --force"

# --- 17. Explicit --force overwrites a file -----------------------------------
new_dest
printf 'old\n' > "$dest/trestle"
(cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --force --version "$ver" >/dev/null 2>&1) || fail "--force overwrite failed"
[ -x "$dest/trestle" ] && [ "$(head -c 2 "$dest/trestle")" = "#!" ] || fail "--force did not replace the file"

# --- 18. Custom --output --------------------------------------------------------
new_dest
(cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --output ./mytrestle --version "$ver" >/dev/null 2>&1) || fail "--output failed"
[ -x "$dest/mytrestle" ] || fail "--output did not create the requested file"
[ ! -e "$dest/trestle" ] || fail "--output also created the default name"

# --- 19. Destination path containing spaces -------------------------------------
new_dest
(cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --output "my trestle" --version "$ver" >/dev/null 2>&1) || fail "--output with spaces failed"
[ -x "$dest/my trestle" ] || fail "--output with spaces did not create the file"
mkdir -p "$dest/sub dir"
(cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --output "sub dir/trestle" --version "$ver" >/dev/null 2>&1) || fail "--output in a spaced directory failed"
[ -x "$dest/sub dir/trestle" ] || fail "--output in a spaced directory did not create the file"

# --- 20. Failed operation leaves no partial destination or staging dir --------
new_dest
(cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/corrupt" sh "$download" --version "$ver" >/dev/null 2>&1) && fail "corrupt download unexpectedly succeeded"
[ ! -e "$dest/trestle" ] || fail "failed download left a partial destination"
[ -z "$(find "$dest" -name '.trestle-download.*' 2>/dev/null)" ] || fail "failed download left a staging directory"

# --- 21. Interrupted operation (SIGTERM) leaves no staging dir -----------------
new_dest
mkdir -p "$fakebin/delay"
cat > "$fakebin/delay/tar" <<'EOF'
#!/bin/sh
sleep 3
exec /bin/tar "$@"
EOF
chmod +x "$fakebin/delay/tar"
( cd "$dest" && PATH="$fakebin/delay:$PATH" TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1 ) &
pid=$!
sleep 1
kill -TERM "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
[ ! -e "$dest/trestle" ] || fail "interrupted download left a partial destination"
[ -z "$(find "$dest" -name '.trestle-download.*' 2>/dev/null)" ] || fail "interrupted download left a staging directory"

# --- 22. No PATH / home / service / configuration / data mutation ---------------
home="$tmp/sandbox-home"
mkdir -p "$home"
before_home=$(find "$home" -mindepth 1 | sort)
# The protected system binary may legitimately exist (a running installed
# Trestle). Snapshot its full state before and require identical state after:
# absence stays absence, a present regular file keeps unchanged size, checksum,
# mtime, mode and owner, and a present symlink keeps its target.
protected=${TRESTLE_TEST_SYSTEM_BINARY:-/usr/local/bin/trestle}
before_sys=$(snapshot "$protected")
new_dest
( cd "$dest" && HOME="$home" TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1 )
after_home=$(find "$home" -mindepth 1 | sort)
[ "$before_home" = "$after_home" ] || fail "download mutated HOME: $after_home"
[ ! -e "$dest/trestle" ] || [ -x "$dest/trestle" ] || fail "destination binary not executable"
after_sys=$(snapshot "$protected")
[ "$before_sys" = "$after_sys" ] || fail "download mutated $protected: before=[$before_sys] after=[$after_sys]"

# --- 23. Packaged test release: real binary runs as ./trestle version ---------
pkg="$tmp/pkg"
mkdir -p "$pkg/v1.0.0-test/trestle_1.0.0-test_linux_amd64"
( cd "$root" && go build -o "$pkg/v1.0.0-test/trestle_1.0.0-test_linux_amd64/trestle" ./cmd/trestle )
tar -C "$pkg/v1.0.0-test" -czf "$pkg/v1.0.0-test/trestle_1.0.0-test_linux_amd64.tar.gz" trestle_1.0.0-test_linux_amd64
(cd "$pkg/v1.0.0-test" && sha256sum trestle_1.0.0-test_linux_amd64.tar.gz > SHA256SUMS)
new_dest
(cd "$dest" && TRESTLE_RELEASE_BASE="file://$pkg" sh "$download" --version v1.0.0-test >/dev/null 2>&1) || fail "packaged release download failed"
"$dest/trestle" version >/dev/null 2>&1 || fail "downloaded executable does not run as ./trestle version"

# --- 24. Protected-binary preservation contract (injectable path) -------------
# Exercises the before/after fingerprint on a disposable path so both initial
# states are covered without writing a sentinel into /usr/local/bin.
sysdir="$tmp/system-bin"
mkdir -p "$sysdir"

# 24a. Absent before -> must remain absent.
absent_path="$sysdir/absent/trestle"
mkdir -p "$(dirname "$absent_path")"
before=$(snapshot "$absent_path")
new_dest
( cd "$dest" && TRESTLE_TEST_SYSTEM_BINARY="$absent_path" TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1 )
after=$(snapshot "$absent_path")
[ "$before" = MISSING ] || fail "absent sentinel was not reported MISSING: $before"
[ "$after" = MISSING ] || fail "download created the protected path $absent_path"

# 24b. Sentinel regular file present -> identical object and metadata.
sink="$sysdir/sink/trestle"
mkdir -p "$(dirname "$sink")"
printf '#!/bin/sh\necho preinstalled\n' > "$sink"
chmod 0755 "$sink"
before=$(snapshot "$sink")
case "$before" in regular\ *) ;; *) fail "sentinel regular file fingerprint: $before" ;; esac
new_dest
( cd "$dest" && TRESTLE_TEST_SYSTEM_BINARY="$sink" TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1 )
after=$(snapshot "$sink")
[ "$before" = "$after" ] || fail "download mutated the sentinel installation: before=[$before] after=[$after]"

# 24c. Sentinel symlink present -> same link target, never dereferenced.
slink="$sysdir/link/trestle"
mkdir -p "$(dirname "$slink")"
ln -s "$sink" "$slink"
before=$(snapshot "$slink")
case "$before" in "symlink ->"*) ;; *) fail "sentinel symlink fingerprint: $before" ;; esac
new_dest
( cd "$dest" && TRESTLE_TEST_SYSTEM_BINARY="$slink" TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1 )
after=$(snapshot "$slink")
[ "$before" = "$after" ] || fail "download mutated the sentinel symlink: before=[$before] after=[$after]"

# 24d. Negative control: content mutation with unchanged length must change the
# checksum component. A fresh regular-file baseline is captured immediately
# before tampering, so the assertion is not conflated with any earlier type.
before_tamper=$(snapshot "$sink")
before_sha=$(fp_field sha256 "$before_tamper")
python3 - "$sink" <<'PY'
import sys
p = sys.argv[1]
d = bytearray(open(p, "rb").read())
d[0] ^= 0x01  # flip one byte; the length is unchanged
open(p, "wb").write(d)
PY
after_tamper=$(snapshot "$sink")
after_sha=$(fp_field sha256 "$after_tamper")
[ "$before_sha" != "$after_sha" ] || fail "content tamper did not change the checksum component"

# 24e. Negative control: mode-only mutation must change the mode component.
before_mode=$(snapshot "$sink")
chmod 0600 "$sink"
after_mode=$(snapshot "$sink")
[ "$(fp_field mode "$before_mode")" != "$(fp_field mode "$after_mode")" ] \
  || fail "mode change was not detected by the fingerprint"

# 24f. Negative control: symlink-target mutation must change the symlink
# fingerprint. The claim is scoped to target preservation, so a repointed link
# is expected to differ.
slink2="$sysdir/link2/trestle"
mkdir -p "$(dirname "$slink2")"
ln -s "$sink" "$slink2"
before_slink=$(snapshot "$slink2")
case "$before_slink" in "symlink ->"*) ;; *) fail "symlink fingerprint: $before_slink" ;; esac
rm "$slink2"
ln -s "$sysdir/other-target" "$slink2"
after_slink=$(snapshot "$slink2")
[ "$before_slink" != "$after_slink" ] || fail "symlink target change was not detected"
rm -f "$slink2" "$sink"

if [ "$failures" -gt 0 ]; then
  echo "download.sh regression: $failures failure(s)" >&2
  exit 1
fi
echo "download.sh regression passed: latest/explicit version, platform mapping, portable verification, fail-closed paths, overwrite safety, atomic staging, no mutation, protected-binary preservation (absent/present/symlink)"