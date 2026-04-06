# Decisions

## Why Postgres is the default runtime

The interesting backend behavior here includes persistence, query filtering, optimistic concurrency, audit history, and retry scheduling. Running the server on Postgres by default makes those concerns first-class instead of optional.

## Why the in-memory store still exists

The workflow logic is already shaped around a repository boundary. Keeping an in-memory implementation makes unit and handler tests cheap while leaving the actual runtime on the persistent path.

## Why header-based actor identity

Authentication is intentionally stubbed so the repo emphasizes workflow behavior, authorization boundaries, and state transitions rather than login plumbing.

## Why explicit state transitions

The interesting engineering here is not a generic rules engine. It is safe, traceable workflow progression with clear authorization and retry behavior. The state machine is kept concrete for that reason.

## Why a poll-based worker

For this repo, a background poller is enough to demonstrate scheduling, retries, and separation between request handling and asynchronous execution without introducing queue infrastructure just for show.

## Why tracing is optional

This repo should look operationally mature without forcing local collector setup. OpenTelemetry is wired in as an opt-in concern so the codebase shows instrumentation patterns while keeping local startup simple.
