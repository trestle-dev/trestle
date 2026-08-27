#!/bin/sh
set -eu
go test ./internal/records -run TestAuthorizationAbuseMatrix -count 10
go test -race ./internal/adminauth ./internal/appauth ./internal/identities ./internal/rules ./internal/records ./internal/files ./internal/server
