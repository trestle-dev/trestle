#!/bin/sh
# Public-script parity gate (CP16).
#
# One canonical source exists for each public script (install.sh, download.sh,
# update.sh in the application repository, sourcing scripts/checksum.sh). The
# standalone copies the website serves are generated deterministically by
# scripts/build-public-scripts.sh, which inlines checksum.sh and removes the two
# repository-only source lines. This gate proves:
#
#   - every script parses under sh -n;
#   - regenerating the standalone public copies reproduces the committed
#     scripts/public/*.sh byte-for-byte (no drifting hand-edited copies);
#   - the website source copies and the generated website-root copies match the
#     canonical standalone public copies exactly;
#   - the deterministic download/install/update regressions pass.
#
# Website paths default to the sibling trestle-dev.github.io repository and can
# be overridden with TRESTLE_WEBSITE_SOURCE / TRESTLE_WEBSITE_OUTPUT.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
failures=0
note() { echo "public scripts: $*"; }
fail() { echo "public scripts FAIL: $*" >&2; failures=$((failures + 1)); }

for f in \
  "$root/scripts/checksum.sh" \
  "$root/install.sh" "$root/download.sh" "$root/update.sh" \
  "$root/scripts/public/install.sh" "$root/scripts/public/download.sh" "$root/scripts/public/update.sh" \
  "$root/scripts/build-public-scripts.sh"; do
  sh -n "$f" || fail "syntax: $f"
done

# Regeneration reproducibility: committed public copies == freshly built ones.
regenerated=$(mktemp -d "${TMPDIR:-/tmp}/trestle-public-parity.XXXXXX")
trap 'rm -rf "$regenerated"' EXIT INT TERM
for name in install download update; do
  {
    cat "$root/scripts/checksum.sh"
    printf '\n'
    sed -e '/^root_dir=\$(CDPATH= cd -- "\$(dirname "\$0")" && pwd)$/d' \
        -e '/^\. "\$root_dir\/scripts\/checksum\.sh"$/d' \
        "$root/$name.sh"
  } > "$regenerated/$name.sh"
  if ! cmp -s "$root/scripts/public/$name.sh" "$regenerated/$name.sh"; then
    fail "committed scripts/public/$name.sh differs from regenerated output; run scripts/build-public-scripts.sh"
  fi
done

# Website parity: source copies and generated website-root copies must match the
# canonical standalone public copies byte-for-byte.
website_source=${TRESTLE_WEBSITE_SOURCE:-"$root/../trestle-dev.github.io"}
website_output=${TRESTLE_WEBSITE_OUTPUT:-"$website_source/public"}
for name in install download update; do
  if [ -f "$website_source/$name.sh" ]; then
    cmp -s "$root/scripts/public/$name.sh" "$website_source/$name.sh" \
      || fail "website source $name.sh differs from the canonical public copy"
  else
    note "website source $name.sh not present at $website_source (parity skipped)"
  fi
  if [ -f "$website_output/$name.sh" ]; then
    cmp -s "$website_source/$name.sh" "$website_output/$name.sh" 2>/dev/null \
      || fail "generated website-root $name.sh differs from the website source copy"
  else
    note "generated website-root $name.sh not present at $website_output (parity skipped)"
  fi
done

# Deterministic behavior regressions against the standalone public scripts.
sh "$root/scripts/test-download.sh" || fail "download.sh regression"
sh "$root/scripts/test-installer.sh" || fail "install.sh regression"
sh "$root/scripts/test-update.sh" || fail "update.sh regression"

if [ "$failures" -gt 0 ]; then
  echo "public-script parity gate: $failures failure(s)" >&2
  exit 1
fi
echo "public-script parity gate passed: syntax, deterministic regeneration, website copies, download/install/update regressions"