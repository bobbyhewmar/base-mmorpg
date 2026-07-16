# PostgreSQL Gameplay Event Outbox

## Objective

Freeze the minimum cross-instance gameplay fanout contract that runs on the existing PostgreSQL deployment. This slice adds no Redis, external queue, broker, remote combat, client fallback, or map change.

The first shipped consumer is an informational remote-target notice. It proves durable production, instance-scoped claiming, retry, delivery, and retention without making a remote player locally interactable.

## Conceptual Reference Boundary

The permitted source study contributed only generic responsibilities: resolve recipients from authoritative presence, separate gameplay mutation from outbound delivery, and make presence entry and exit explicit. Its process-local implementation, code, packet shapes, schemas, identifiers, and assets were not reused.

This project translates those concepts into its own PostgreSQL outbox, durable session ownership routing, command correlation, stable event ids, and asynchronous dispatcher.

## Durable Event Contract

`gameplay_event_outbox` stores one delivery intent per exact destination instance. Every row contains:

- monotonic `event_id` from `BIGSERIAL`
- globally unique `idempotency_key`
- versioned `event_type`
- bounded JSONB `payload_json`
- exact `target_server_instance_id`
- optional target region, session, and character metadata
- `created_at` and `available_at`
- `claimed_at`, claim owner, and claim deadline
- `delivered_at`
- retry count and summarized last error
- optional dead-letter timestamp

The exact instance destination is mandatory in this first slice. Region, session, and character are routing constraints and observability metadata, not alternative sources of authority. A future regional broadcast must materialize one delivery row per destination instance; a single row must never be consumed once and mistaken for a multi-instance broadcast.

Payloads are limited to 64 KiB and must contain valid JSON. Logs never include the payload or sensitive credentials.

## Idempotency And Command Atomicity

An idempotency key identifies one immutable event intent. Recreating the same key with the same type, payload, and destination is a no-op that returns the existing event id. Reusing the key with different immutable content is a storage conflict.

The current producer derives the key from the authoritative session and command sequence:

`gameplay-command/{session_id}/{command_seq}/remote-target-notice`

For the remote-target path, finalizing `gameplay_command_records` and inserting the outbox row happen in one PostgreSQL transaction. If event validation or insertion fails, the command outcome update rolls back. An identical command replay reads the stored outcome and does not run the producer again. A conflicting replay remains `sequence.conflicting_replay`.

The selected target itself remains runtime-only and is not persisted. The durable row represents only the notification intent created by the rejected remote interaction.

## Claim And Delivery

Each server process has a unique worker id under its stable `server_instance_id`. The background dispatcher:

1. polls independently from the gameplay request pipeline
2. selects only undelivered, non-dead-letter events for its exact instance whose retry deadline is ready
3. uses a short transaction with `FOR UPDATE SKIP LOCKED`
4. writes a bounded claim lease and returns the claimed rows
5. revalidates the target's current durable ownership and matching ready local runtime
6. delivers the supported event
7. marks the row delivered only while the worker still owns the live claim

Concurrent workers cannot claim the same live row. An expired claim may be reclaimed.

PostgreSQL time owns claim, delivery, retry, and dead-letter deadlines so clock skew between backend instances cannot steal or retain a claim early.

Transport is at-least-once. Every delivery carries the stable monotonic `event_id`; the live runtime keeps a bounded set of successfully delivered ids, and the current notice has no gameplay mutation, so repeating it is semantically idempotent. A future state-changing consumer must persist its consumer receipt in the same transaction as its mutation before it can use this outbox.

## First Event: Remote Target Notice

When `select_target` references a known player whose active owner is another instance:

- the actor still receives `presence.target_remote`
- the actor's local target does not change
- no PvP damage, party invite, clan invite, selection success, or remote runtime fallback occurs
- the command outcome and one `presence.remote_target_notice.v1` event commit atomically
- the target instance revalidates exact character and session ownership before emitting `presence_notice`

The notice contains only the outbox event id, notice type, actor character id, stable reason code, and server timestamps. It is lifecycle information, not snapshot or delta authority. The browser does not derive target, presence, or combat state from it.

Remote PvP remains unsupported and continues to return `presence.target_remote`. Party, clan, chat, entity, and movement fanout are not added by this first producer.

## Retry, Dead Letter, And Retention

Delivery failures store a bounded machine-oriented error summary, increment `retry_count`, release the claim, and set an exponential retry deadline. After the configured maximum, the row receives `dead_lettered_at` and is no longer claimable. A failed event never blocks server startup or the gameplay command pipeline.

Retention deletes only rows whose `delivered_at` is older than the configured retention window. Pending, claimed, retrying, and dead-letter rows are never removed by delivered-event retention.

Defaults:

- poll interval: 250 ms
- claim lease: 5 seconds
- first retry delay: 500 ms
- maximum attempts: 5
- delivered retention: 24 hours
- cleanup interval: 10 minutes
- claim batch: 32

## Observability

`l2bg_gameplay_outbox_events_total{result=...}` and structured `gameplay_outbox` logs cover:

- `produced`
- `claimed`
- `delivered`
- `failed`
- `retried`
- `dead_lettered`
- `expired` for retention deletion

Logs include event id, event type, destination instance, retry count, and a bounded failure code when applicable. They exclude payloads, attach tokens, access tokens, and account credentials.

## Memory Adapter

The memory adapter shares the same semantics for deterministic tests:

- monotonic event ids
- immutable idempotency keys
- exact instance routing
- mutex-serialized claim leases
- retry and dead-letter state
- delivered-only retention
- atomic command outcome plus event creation under one lock

Two `Store` wrappers may share one memory backend to simulate separate server instances without adding infrastructure.

## Non-Goals

- remote damage or cross-instance combat transactions
- region-wide broadcast delivery
- cross-instance movement or entity replication
- party, clan, or chat fanout
- Redis, broker, external queue, or event sourcing
- admin panel or manual replay UI
- payload logging
- client-authored delivery success

## Invariants

- only the exact destination instance may claim an event
- claim concurrency does not duplicate live delivery
- delivered marking requires the current unexpired worker claim
- identical command replay cannot duplicate the event
- conflicting command replay remains rejected
- a remote notice never changes the actor's target or enables remote gameplay
- failed delivery cannot fail the gameplay server loop
- retention removes only old delivered rows
- PostgreSQL remains durable truth for outbox state; runtime memory is only the live dispatcher projection
