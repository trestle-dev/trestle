#!/bin/sh
set -eu
go test ./internal/records -run 'TestBoundedConcurrentRecordReadLoad|TestRecordResourceLimitsFailControlled' -count 3
go test -race ./internal/records -run TestBoundedConcurrentRecordReadLoad -count 1
go test ./internal/server -run TestSlowHeaderClientIsDisconnected -count 10
