CREATE TABLE IF NOT EXISTS requests (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    target_resource TEXT NOT NULL,
    justification TEXT NOT NULL,
    requester_id TEXT NOT NULL,
    assigned_reviewer_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    reminder_count INTEGER NOT NULL DEFAULT 0,
    execution_attempts INTEGER NOT NULL DEFAULT 0,
    last_execution_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    submitted_at TIMESTAMPTZ NULL,
    approved_at TIMESTAMPTZ NULL,
    reviewed_at TIMESTAMPTZ NULL,
    executed_at TIMESTAMPTZ NULL,
    last_reminded_at TIMESTAMPTZ NULL,
    next_execution_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS request_reviews (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    decision TEXT NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS request_audit_logs (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL,
    actor_role TEXT NOT NULL,
    action TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    scope TEXT NOT NULL,
    key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    request_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (scope, key)
);

CREATE TABLE IF NOT EXISTS job_runs (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    executed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_requests_requester_status ON requests (requester_id, status);
CREATE INDEX IF NOT EXISTS idx_requests_reviewer_status ON requests (assigned_reviewer_id, status);
CREATE INDEX IF NOT EXISTS idx_requests_due_execution ON requests (status, next_execution_at);
CREATE INDEX IF NOT EXISTS idx_requests_submitted_at ON requests (status, submitted_at);
CREATE INDEX IF NOT EXISTS idx_audit_request_created ON request_audit_logs (request_id, created_at);
