# Multi-Backend Fanout Validation

## Purpose

This runbook exercises the PostgreSQL outbox, durable receipts, ownership fencing, regional chat, and player projections with two real backend processes. It is an operational validation profile, not a replacement for the default development stack.

## Topology

The `multi-backend` Compose profile starts:

- `backend-a` as `backend-multi-a` on host port `18081`
- `backend-b` as `backend-multi-b` on host port `18082`
- `frontend-a` on host port `15173`, proxying backend A
- `frontend-b` on host port `15174`, proxying backend B
- the existing shared PostgreSQL service

Both backends use the same database and different durable `server_instance_id` values. The ordinary `backend` and `frontend` services and their ports remain unchanged.

## Canonical Scenario

From the repository root, with Docker Desktop running:

```powershell
npm run e2e:multi-backend
```

The runner builds the shared profile images once, starts the two backend/frontend pairs, waits for both health endpoints, runs the Playwright scenario, and removes only the profile services afterward. It preserves the default development stack.

The scenario verifies:

- separate ownership and fencing for two characters attached to different instances
- projection A to B and B to A
- local and remote regional chat without client-authored success
- a burst of movement/projection publication
- backend-B stop, projection TTL removal on A, retry/dead-letter behavior, and restart recovery
- an intentionally delayed old projection cannot cross the tombstone/version barrier and resurrect an entity
- reconnect receives a newer ownership fence and resumes command sequencing from durable `next_command_seq`
- an event addressed to the previous recipient fence cannot cross takeover even when the durable session id is reused
- queue, delivery-delay, outbox, receipt, retry, dead-letter, and despawn measurements

The run is destructive only to the test accounts/characters it creates. It does not reset the shared database.

## Queue Pressure

The profile intentionally sets `L2BG_REGION_PROJECTION_QUEUE_SIZE=4`. Movement bursts therefore exercise the bounded publisher and its latest-per-source coalescing buffer. Inspect either backend's `/metrics` endpoint for:

- `l2bg_region_projection_queue_events_total{result="enqueued|dequeued|coalesced|dropped"}`
- `l2bg_region_projection_queue_depth`
- `l2bg_region_projection_queue_capacity`
- `l2bg_region_projection_queue_coalesced_depth`
- `l2bg_region_projection_delivery_delay_seconds_sum`
- `l2bg_region_projection_delivery_delay_seconds_count`
- `l2bg_region_projection_delivery_delay_seconds_max`

`coalesced` means an older not-yet-persisted snapshot for the same source ownership was superseded in memory. `dropped` means the bounded queue and bounded coalescing spill were both full. Neither is local success: the two-second heartbeat republishes current authoritative state and the six-second TTL removes stale projection state.

## Measured Baseline

On 2026-07-16, a cold local Docker Desktop run on the development workstation completed in approximately eight minutes of Playwright execution and reported:

```text
projection_events=166
chat_events=4
receipts=141
retry_count=87
dead_letters=29
despawns=1
delivery_delay_avg_ms=3468.1
delivery_delay_max_ms=21413.594
```

The retry and dead-letter counts are expected in this fault scenario because backend B is intentionally unavailable while exact-owner events continue to be produced. The result is a reproducible functional baseline, not a production latency SLO. The maximum delay shows that obsolete durable projection rows can accumulate while an owner is down; safe supersession/compaction of obsolete undelivered projection snapshots is the recommended next slice before finer interest management.

## Failure Interpretation

- A stale-owner chat or notice must retry/dead-letter and must not appear after reconnect under a newer fence.
- A current heartbeat after recovery must restore the latest visual projection even if bounded queue pressure dropped an intermediate update.
- A projection with an older source `(fence, version)` must never recreate an entity after despawn/TTL tombstone.
- A dead-letter is durable evidence of failed delivery, not socket or HUD success.
- Exactly-once socket delivery is not claimed; durable receipts plus browser event-id dedup cover supported at-least-once redelivery semantics.
