#!/bin/sh
set -eu
go test ./internal/store -run 'TestUpgradeFromEveryRetainedSchemaVersion|TestInterruptedUpgradePreservesPriorVersionAndData|TestRefusesFutureSchema' -count 5
