#!/bin/sh
set -eu
go test ./internal/backup -run 'TestBackupRestoreDrill|TestRestore' -count 5
"$(dirname "$0")/test-update.sh"
