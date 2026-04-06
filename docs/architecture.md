# Architecture

## Overview

The service is organized around a narrow workflow domain rather than generic CRUD handlers:

- `cmd/server`: process bootstrap
- `internal/domain`: entities and status model
- `internal/application`: workflow rules, authorization, idempotency, and execution behavior
- `internal/repository/postgres`: runtime persistence layer
- `internal/repository/memory`: test-only store for fast service checks
- `internal/transport/http`: Gin handlers
- `internal/platform/observability`: optional OpenTelemetry bootstrap
- `internal/jobs`: periodic worker

## Request lifecycle

1. An employee creates a draft.
2. The employee submits the request.
3. The service assigns the default reviewer and moves the request into `pending_review`.
4. The reviewer approves or rejects it.
5. Approved requests are queued for execution.
6. The worker simulates provisioning and either marks the request `executed` or schedules a retry as `execution_failed`.

## Operational model

- Mutating workflow actions require an `Idempotency-Key`.
- Every meaningful transition appends an audit log entry.
- Request updates use optimistic concurrency via the `version` field.
- The worker is poll-based and reuses the same service layer as the HTTP API.
- Tracing is opt-in and instruments HTTP requests, migrations, repository calls, and asynchronous worker/execution paths.

## Persistence split

The runtime server uses Postgres and runs embedded migrations at startup. The in-memory store remains in-tree only to keep service and transport tests fast and deterministic without requiring a live database.
