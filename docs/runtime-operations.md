# Runtime and Operations

## Environments

Use a small, repeatable environment ladder:

- local development
- shared staging
- production

Keep build, deploy, and migration paths the same across environments whenever possible.

## Linux Production Baseline

- run the Go backend as a single packaged service per environment
- place PgBouncer in front of PostgreSQL
- centralize structured logs
- export metrics and traces through OpenTelemetry-compatible tooling
- keep secrets outside the repository
- keep transactional email provider credentials and webhook secrets in managed secret storage
- operate a verified sending subdomain for transactional email

## Minimum Operational Signals

Track at least these metrics from the start:

- request rate, latency, and error rate
- command validation failures by reason
- transaction retry rate
- database connection pool pressure
- WebSocket connection count
- queue depth and job age
- login success and failure rate
- registration success and failure rate
- character-creation success and failure rate
- character-name collision and profanity-rejection rate
- session attach success and failure rate
- active sessions and active characters
- region occupancy and region transfer rate
- pathfinding latency, budget-exceeded count, and movement rejection count by reason
- click-to-move perceived latency and prediction-correction frequency when client telemetry exists
- combat action rate and loot action rate
- transactional email send success and failure counts
- transactional email lag from intent creation to provider send
- webhook verification failures and event ingest lag
- suspicious auth and attach attempts
- price-mismatch or client-trust rejection events for economy paths when they are introduced
- client-supplied path, waypoint, collision, or geodata override rejection events when movement hardening is introduced

## Current Backend Signals

The current backend now exposes a local `/metrics` endpoint in Prometheus text format and emits structured JSON logs for the most critical synchronous paths.

Current metrics include:

- `l2bg_http_requests_total`
- `l2bg_http_errors_total`
- `l2bg_http_request_duration_seconds_count`
- `l2bg_http_request_duration_seconds_sum`
- `l2bg_ws_attach_attempts_total`
- `l2bg_websocket_connections_active`
- `l2bg_attached_sessions_active`
- `l2bg_session_ownership_events_total{result}`
- `l2bg_region_occupancy`
- `l2bg_gameplay_commands_total`
- `l2bg_gameplay_command_duration_seconds_count`
- `l2bg_gameplay_command_duration_seconds_sum`
- `l2bg_gameplay_outbound_messages_total`
- `l2bg_gameplay_rejects_total`
- `l2bg_db_errors_total`

Current structured logs cover:

- HTTP request completion with method, path, status, duration, and remote IP
- WebSocket attach outcomes with result and reason code
- durable ownership acquire, renew, replace, conflict, expiry, stale-owner reject, and release events with instance and fence context
- gameplay command completion with command type, sequence, result, duration, and reject reason when present
- critical persistence failures with operation and store mode

This is the minimum observability slice for the current monolith. It is intentionally small and local to the backend application layer so we can evolve dashboards and OpenTelemetry export later without changing command authority boundaries.

## Release Gates

Do not ship a release unless it has:

- passing automated tests
- schema migration review
- rollback or forward-fix plan
- dashboard coverage for critical request paths
- alert thresholds for sustained failures
- tested transactional email configuration in staging when email-touching code changes

## Resilience Practices

- prefer backpressure over uncontrolled retries
- bound worker concurrency
- fail closed on authorization and integrity checks
- fail closed when client input attempts to supply authoritative economy or gameplay results
- fail closed when client input attempts to supply authoritative paths, waypoints, collision results, or geodata overrides
- rehearse database restore procedures
- measure p95 and p99 latencies, not only averages
- verify external webhook signatures before accepting provider events
- rate-limit registration, login, recovery, and gameplay-session attach paths

## Recovery Priorities

Recover in this order:

1. gameplay command path
2. live session connectivity
3. account and support tooling
4. non-critical background jobs
5. non-critical notification delivery

## Capacity Planning Bias

Capacity-plan from measured command volume, not from total registered users. Re-run load tests when any of these change materially:

- command mix
- region density
- geodata complexity and pathfinding budget
- acceptable prediction leash and correction rate
- snapshot cadence
- observability overhead
- deployment topology
