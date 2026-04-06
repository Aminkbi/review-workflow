# Failure Scenarios

## Duplicate submit

If the client retries `submit` with the same `Idempotency-Key`, the service returns the current request instead of creating a second transition.

## Duplicate approve or reject

Review actions are protected by idempotency keys and optimistic locking. A replay of the same action returns the current request state.

## Concurrent review attempts

Request updates carry an expected `version`. If two reviewers race on the same request, only one update should win.

## Execution failure

If provisioning fails, the request moves to `execution_failed`, the error is captured on the request, and the worker schedules another attempt until the retry budget is exhausted.

## Unauthorized visibility

Employees can only view their own requests. Reviewers can act on assigned requests. Admins can inspect everything.

