# Trestle incident tracker

This is the CP21 dogfood application. It is a small Go web frontend backed only
by Trestle's published HTTP APIs. It does not import Trestle internals or read
the SQLite database.

## Run the initialized example

From the packaged example directory, start Trestle:

```sh
./trestle --data-dir ./data --listen 127.0.0.1:8090
```

In another terminal, start the example frontend:

```sh
./incident-tracker
```

Open `http://127.0.0.1:8091`. The application-user credentials are:

```text
reporter@example.com
reporter7
```

The Trestle dashboard is at `http://127.0.0.1:8090`. Its initialized
administrator credentials are:

```text
admin@example.com
mudblood
```

The example contains incidents, related timeline updates, an attached runbook,
access rules, a scoped service identity, realtime journal entries, audit facts,
a completed no-op job, a deliberately non-delivering webhook target, an AWS
Lambda target demonstrating missing-credential retry diagnostics, and a tested
backup archive.

## Recreate from scratch

Build Trestle and the two example commands, start Trestle against an empty data
directory, then run the seed command:

```sh
go build -o trestle ./cmd/trestle
go build -o incident-tracker ./examples/incident-tracker/cmd/app
go build -o seed-incident-tracker ./examples/incident-tracker/cmd/seed

./trestle --data-dir ./example-data --listen 127.0.0.1:8090
./seed-incident-tracker -url http://127.0.0.1:8090
```

The seed command is intentionally an API client. It fails on the first
unexpected response and prints a verification summary only after replaying an
event, checking application-user rules, downloading an attachment, inspecting
audit/jobs, creating a backup, and submitting that archive to restore preflight.

