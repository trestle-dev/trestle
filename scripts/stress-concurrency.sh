#!/bin/sh
set -eu
count=${TRESTLE_STRESS_COUNT:-20}
go test -race ./internal/jobs ./internal/store ./internal/records ./internal/events -count "$count"
