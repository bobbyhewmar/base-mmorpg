# Local Docker Compose

This setup is intentionally limited to local development and lightweight homologation. It is not a production topology decision.

## Services

- `frontend`: Vite dev server on port `5173`
- `backend`: Go HTTP/WebSocket server on port `8080`
- `postgres`: PostgreSQL on port `5432` with a named volume for local persistence
- `db-init`: one-shot schema bootstrap service that validates and creates the minimum PostgreSQL tables before the backend starts

## Start

1. Optionally copy `.env.example` to `.env` and adjust values.
2. Run `docker compose up -d --build`.

The browser entrypoint is `http://localhost:5173`.

Startup order:

1. `postgres` becomes healthy
2. `db-init` applies the idempotent bootstrap SQL and validates the minimum schema
3. `backend` starts only after `db-init` exits successfully
4. `frontend` starts and proxies browser API traffic to the backend

## Stop

- `docker compose down`

## Reset

- `docker compose down -v`

This removes the local PostgreSQL volume created by Compose.

## Validate Schema

- Check the bootstrap logs: `docker compose logs db-init`
- Confirm the minimum tables exist:
  `docker compose exec postgres psql -U "$L2BG_POSTGRES_USER" -d "$L2BG_POSTGRES_DB" -c "SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename IN ('schema_bootstrap', 'accounts', 'account_credentials', 'account_sessions', 'characters', 'gameplay_sessions', 'gameplay_session_ownerships', 'gameplay_command_records', 'character_hotbar_loadouts', 'action_logs', 'storage_transfer_records') ORDER BY tablename;"`

## Internal Economy Audit Queries

These endpoints are intentionally disabled by default and are not part of the gameplay API surface.

1. Set `L2BG_INTERNAL_AUDIT_ENABLED=true`
2. Set `L2BG_INTERNAL_AUDIT_TOKEN` to a non-empty local secret
3. Restart with `docker compose up -d --build`

Example queries:

- `curl -H "X-Internal-Audit-Token: local-audit-secret" "http://localhost:8080/internal/economy/events?character_id=char_123&limit=25"`
- `curl -H "X-Internal-Audit-Token: local-audit-secret" "http://localhost:8080/internal/economy/events?action_type=vendor_buy&from=2026-07-12T00:00:00Z&to=2026-07-12T23:59:59Z"`
- `curl -H "X-Internal-Audit-Token: local-audit-secret" "http://localhost:8080/internal/economy/events?item_instance_id=item_123"`
- `curl -H "X-Internal-Audit-Token: local-audit-secret" "http://localhost:8080/internal/economy/warehouse-transfers?character_id=char_123&transfer_type=warehouse_deposit"`
- `curl -H "X-Internal-Audit-Token: local-audit-secret" "http://localhost:8080/internal/economy/trades?character_id=char_123&limit=50&offset=0"`

Notes:

- `limit` defaults to `50` and is capped at `200`
- `offset` defaults to `0`
- `from` and `to` accept RFC3339 or unix milliseconds
- `/internal/economy/warehouse-transfers` requires `character_id`
- `/internal/economy/trades` requires `character_id`

## Environment variables

- `L2BG_FRONTEND_PORT`: published frontend port
- `L2BG_BACKEND_PORT`: published backend port
- `L2BG_POSTGRES_PORT`: published PostgreSQL port
- `VITE_L2BG_API_BASE_URL`: frontend API base URL; default `/api`
- `L2BG_VITE_PROXY_TARGET`: Vite proxy target inside the Compose network
- `L2BG_BACKEND_ADDR`: backend bind address inside the container
- `L2BG_DATABASE_URL`: backend database URL used by the Go backend for durable PostgreSQL-backed flows
- `L2BG_PUBLIC_WS_URL`: public WebSocket URL returned by `world_enter`
- `L2BG_ALLOWED_ORIGINS`: optional comma-separated HTTP CORS allowlist for direct browser-to-backend access
- `L2BG_ACCESS_TOKEN_TTL`: optional account-session TTL duration such as `2h`
- `L2BG_AUTH_RATE_LIMIT_MAX_ATTEMPTS`: optional auth rate-limit override for local or automated environments
- `L2BG_AUTH_RATE_LIMIT_WINDOW`: optional auth rate-limit window duration such as `1m`
- `L2BG_ATTACH_RATE_LIMIT_MAX_ATTEMPTS`: optional attach rate-limit override for local or automated environments
- `L2BG_ATTACH_RATE_LIMIT_WINDOW`: optional attach rate-limit window duration such as `1m`
- `L2BG_INTERNAL_AUDIT_ENABLED`: enable the internal read-only economy audit endpoints
- `L2BG_INTERNAL_AUDIT_TOKEN`: required token for `X-Internal-Audit-Token` when internal audit is enabled
- `L2BG_TEST_DATABASE_URL`: optional backend test override used by the Compose validation flow
- `L2BG_SERVER_INSTANCE_ID`: stable unique id for this backend process; every concurrently running instance must use a different value
- `L2BG_SESSION_LEASE_DURATION`: durable gameplay ownership lease duration; default `30s`
- `L2BG_SESSION_LEASE_RENEW_INTERVAL`: idle WebSocket renewal cadence and must be shorter than the lease; default `10s`
- `L2BG_SESSION_ATTACH_TOKEN_TTL`: rolling attach-credential deadline maintained while ownership renews; default `5m`
- `L2BG_AUTH_SOCIAL_GOOGLE_CLIENT_ID`: optional Google OAuth client id for backend-managed social auth begin
- `L2BG_AUTH_SOCIAL_GOOGLE_CLIENT_SECRET`: optional Google OAuth client secret reserved for future callback/token exchange work; do not commit real values
- `L2BG_AUTH_SOCIAL_GOOGLE_REDIRECT_URL`: optional Google OAuth redirect URL handled by the deployed auth surface
- `L2BG_AUTH_SOCIAL_FACEBOOK_CLIENT_ID`: optional Facebook OAuth client id for backend-managed social auth begin
- `L2BG_AUTH_SOCIAL_FACEBOOK_CLIENT_SECRET`: optional Facebook OAuth client secret reserved for future callback/token exchange work; do not commit real values
- `L2BG_AUTH_SOCIAL_FACEBOOK_REDIRECT_URL`: optional Facebook OAuth redirect URL handled by the deployed auth surface
- `L2BG_POSTGRES_DB`: PostgreSQL database name created automatically by the PostgreSQL image
- `L2BG_POSTGRES_USER`: PostgreSQL user
- `L2BG_POSTGRES_PASSWORD`: PostgreSQL password

## Notes

- The frontend uses a small Vite proxy so browser HTTP traffic can reach the backend through the Compose network without changing API contracts.
- PostgreSQL creates the configured database automatically on first boot through the standard image behavior.
- The schema bootstrap is intentionally kept out of the backend runtime and runs as a dedicated, idempotent Compose service.
- The backend already uses PostgreSQL-backed adapters when `L2BG_DATABASE_URL` is configured and falls back to the in-memory adapter only when that variable is intentionally omitted.
- Auth and attach rate limits still default inside the backend; the new env overrides are only needed when local automation intentionally generates more attempts than the default budget.
- PostgreSQL-backed `gameplay_session_ownerships` is the multi-instance session authority. Do not reuse one `L2BG_SERVER_INSTANCE_ID` across live backend replicas.
- The browser-level online E2E flow is documented in `docs/e2e-docker-online.md`.
