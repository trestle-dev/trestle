#!/bin/sh
set -eu

go test ./...
go test -race ./...
go vet ./...
./scripts/test-browser-quality.sh
./scripts/audit-contracts.sh
if command -v docker >/dev/null 2>&1; then
  ./scripts/test-proxies.sh
else
  echo "docker unavailable: running retained trusted-proxy unit matrix"
  go test ./internal/server -run 'TestForwarded|TestTrusted' -count 10
fi
./scripts/test-upgrades.sh
./scripts/test-recovery-drills.sh
./scripts/test-security-abuse.sh
./scripts/test-load.sh
TRESTLE_STRESS_COUNT=5 ./scripts/stress-concurrency.sh
TRESTLE_FUZZ_TIME=2s ./scripts/fuzz-boundaries.sh
./scripts/test-release.sh
./scripts/test-release-contract.sh
./scripts/test-release-reproducible.sh
./scripts/test-installer.sh
./scripts/test-update.sh
./scripts/test-download.sh
./scripts/test-public-scripts.sh
./scripts/test-quickstart.sh
echo "release-candidate matrix passed"
