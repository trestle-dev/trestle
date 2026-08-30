#!/bin/sh
set -eu

node scripts/check-dashboard-quality.mjs
node scripts/test-database-setup.mjs
node --check internal/web/public/assets/js/script.js
go test ./internal/server ./internal/web
