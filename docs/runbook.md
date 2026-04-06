# Runbook

## Start

```bash
docker compose up -d postgres
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/review_workflow?sslmode=disable'
go run ./cmd/server
```

## Verify health

```bash
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/readyz
```

## Verify worker execution

Use a target resource that contains `fail` to force execution retries:

```bash
curl -s http://localhost:8080/v1/requests \
  -H 'Content-Type: application/json' \
  -H 'X-Actor-Id: alice' \
  -H 'X-Actor-Role: employee' \
  -d '{"type":"system_access","target_resource":"fail-crm","justification":"Testing retries"}'
```

After submit and approve, inspect the request and audit log endpoints to confirm the retry path and recorded failure.

## Optional tracing

To emit OTLP traces to a local collector:

```bash
export OTEL_ENABLED=true
export OTEL_SERVICE_NAME=review-workflow
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
```

## Notes

- The server requires `DATABASE_URL`.
- Startup applies the embedded SQL migration set automatically.
- The in-memory repository is still used in tests, not in the default runtime.
- Tracing stays disabled unless `OTEL_ENABLED=true`.
