#!/bin/sh
set -eu
duration=${TRESTLE_FUZZ_TIME:-5s}
go test ./internal/query -run '^$' -fuzz '^FuzzParseCompile$' -fuzztime "$duration"
go test ./internal/rules -run '^$' -fuzz '^FuzzRuleValidationAndEvaluation$' -fuzztime "$duration"
go test ./internal/webhooks -run '^$' -fuzz '^FuzzTargetURLValidation$' -fuzztime "$duration"
go test ./internal/functions -run '^$' -fuzz '^FuzzLambdaTargetValidation$' -fuzztime "$duration"
