#!/bin/sh
set -eu

site=../trestle-dev.github.io
actual=$(rg -o 'TRESTLE_[A-Z0-9_]+' internal/config/config.go | sort -u)
documented=$({ rg -o 'TRESTLE_[A-Z0-9_]+' README.md "$site/content"; } | sed 's/.*://' | sort -u)
# POSIX shells cannot portably feed two generated streams to comm, so compare
# each production variable directly.
for variable in $actual; do
  printf '%s\n' "$documented" | grep -qx "$variable" || {
    echo "undocumented configuration variable: $variable" >&2
    exit 1
  }
done

go mod verify >/dev/null
go list -m all >/dev/null

if rg -n '—|PocketBase|CP23 planned|CP00-CP22 are implemented' README.md docs "$site/content" "$site/templates"; then
  echo "stale or prohibited public wording found" >&2
  exit 1
fi

(cd "$site" && node scripts/check-site.mjs)
echo "contract audit: configuration, stale-language and public-link checks passed"
