# Review Workflow

A Go service for internal access approvals.

Employees create requests, reviewers approve or reject them, and approved work is executed asynchronously with a full audit trail.

## Features

- Gin-based REST API with header-based actor identity
- Embedded UI at `/` with no frontend build step
- Explicit workflow state machine
- Idempotent submit, approve, and reject actions
- Audit trail for every transition
- Background worker for reminders and execution retries
- Postgres-backed runtime repository with embedded SQL migrations
- Optional OpenTelemetry tracing for Gin, Postgres access, migrations, worker passes, and execution flow
- In-memory repository retained for tests and isolated service checks

## Workflow

`draft -> pending_review -> approved | rejected -> executed | execution_failed`

Core behaviors:

- requester creates and submits a draft
- reviewer approves or rejects assigned requests
- approved requests are executed by a background worker
- execution failures are retried with bounded attempts
- every transition is persisted and appended to the audit log

## Architecture

- `cmd/server`: bootstrap, config, DB connection, tracing init
- `internal/domain`: workflow entities and status types
- `internal/application`: business rules, authorization, idempotency, retries
- `internal/repository/postgres`: runtime persistence layer
- `internal/repository/memory`: fast test double
- `internal/jobs`: periodic worker for reminders and execution
- `internal/platform/observability`: optional OTel setup
- `internal/transport/http`: Gin API and embedded UI

## API

Headers on every request:

- `X-Actor-Id`
- `X-Actor-Role` with `employee`, `reviewer`, or `admin`

Headers on mutating workflow actions:

- `Idempotency-Key`

Endpoints:

- `GET /`
- `POST /v1/requests`
- `GET /v1/requests`
- `GET /v1/requests/{id}`
- `POST /v1/requests/{id}/submit`
- `POST /v1/requests/{id}/approve`
- `POST /v1/requests/{id}/reject`
- `POST /v1/requests/{id}/execution-result`
- `GET /v1/requests/{id}/audit`
- `GET /healthz`
- `GET /readyz`

## Run locally

```bash
docker compose up -d postgres
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/review_workflow?sslmode=disable'
go test ./...
go run ./cmd/server
```

The server starts on `:8080` by default.

Open `http://localhost:8080/` to use the built-in dashboard.

## UI preview

The project includes a lightweight embedded UI for local demo flows and quick manual inspection.

![Dashboard view](docs/assets/ui-dashboard.png)

![Reviewer view](docs/assets/ui-reviewer.png)

Environment variables:

- `HTTP_ADDR` default `:8080`
- `DATABASE_URL` required
- `DEFAULT_REVIEWER_ID` default `reviewer-1`
- `REMINDER_AFTER` default `30m`
- `EXECUTION_RETRY_BASE` default `15s`
- `EXECUTION_MAX_ATTEMPTS` default `3`
- `WORKER_POLL_INTERVAL` default `10s`
- `WORKER_BATCH_SIZE` default `25`
- `OTEL_ENABLED` default `false`
- `OTEL_SERVICE_NAME` default `review-workflow`
- `OTEL_EXPORTER_OTLP_ENDPOINT` required only when tracing is enabled
- `OTEL_EXPORTER_OTLP_INSECURE` default `false`
- `OTEL_SAMPLE_RATIO` default `1.0`

## Notes

- `GET /` for the embedded operator UI
- request submission and review actions for idempotency behavior
- audit history for every transition
- worker-driven execution retries by using a target resource containing `fail`
- optional tracing hooks across HTTP, migrations, DB calls, and background work

## Example flow

Create a draft:

```bash
curl -s http://localhost:8080/v1/requests \
  -H 'Content-Type: application/json' \
  -H 'X-Actor-Id: alice' \
  -H 'X-Actor-Role: employee' \
  -d '{"type":"system_access","target_resource":"crm","justification":"Need support access"}'
```

Submit it:

```bash
curl -s -X POST http://localhost:8080/v1/requests/<request-id>/submit \
  -H 'X-Actor-Id: alice' \
  -H 'X-Actor-Role: employee' \
  -H 'Idempotency-Key: submit-1'
```

Approve it:

```bash
curl -s -X POST http://localhost:8080/v1/requests/<request-id>/approve \
  -H 'Content-Type: application/json' \
  -H 'X-Actor-Id: reviewer-1' \
  -H 'X-Actor-Role: reviewer' \
  -H 'Idempotency-Key: approve-1' \
  -d '{"comment":"Approved for on-call rotation"}'
```

## Persistence note

The default server uses Postgres through [main.go](/home/aminkhanbabaei/personal/review-workflow/cmd/server/main.go) and applies the SQL files embedded from [db/migrations](/home/aminkhanbabaei/personal/review-workflow/db/migrations).

## Observability

Tracing is optional and disabled by default, so the project runs normally without a collector.

Example:

```bash
export OTEL_ENABLED=true
export OTEL_SERVICE_NAME=review-workflow
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
```

When enabled, the service emits spans for incoming Gin requests, Postgres repository operations, migration execution, worker passes, and simulated execution attempts.

## Reliability notes

- duplicate submit/approve/reject requests are protected by idempotency keys
- concurrent updates are guarded by optimistic version checks
- execution failure is captured on the request and retried asynchronously
- audit history preserves who did what and when
- unauthorized actors cannot inspect or mutate unrelated requests

## Docs

- [Architecture](docs/architecture.md)
- [Decisions](docs/decisions.md)
- [Failure scenarios](docs/failure-scenarios.md)
- [Runbook](docs/runbook.md)
