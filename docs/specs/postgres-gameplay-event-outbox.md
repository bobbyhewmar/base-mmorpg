# PostgreSQL Gameplay Event Outbox

## Objective

Freeze the minimum cross-instance gameplay fanout contract that runs on the existing PostgreSQL deployment. This slice adds no Redis, external queue, broker, remote combat, client fallback, or map change.

The shipped consumers are an informational remote-target notice, canonical remote whisper/region chat, party/clan notices, and exact-recipient regional player projections. They prove durable production, instance-scoped claiming, retry, delivery, receipts, and retention without making a remote player locally authoritative or introducing remote combat.

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
- optional projection-stream metadata for `presence.region_player_projection.v1` (`projection_source_character_id`, source fence/version, recipient fence, action)
- optional `superseded_at` and `superseded_by_event_id` when a newer projection row obsoletes an older undelivered row
- retry count and summarized last error
- optional dead-letter timestamp

The exact instance destination is mandatory. Region, session, and character are routing constraints and observability metadata, not alternative sources of authority. Region chat materializes one exact-destination delivery row per remote recipient resolved from active ownership; a single row is never consumed once and mistaken for a multi-recipient or multi-instance broadcast.

Payloads are limited to 64 KiB and must contain valid JSON. Logs never include the payload or sensitive credentials.

## Idempotency And Command Atomicity

An idempotency key identifies one immutable event intent. Recreating the same key with the same type, payload, and destination is a no-op that returns the existing event id. Reusing the key with different immutable content is a storage conflict.

Producers derive keys from the authoritative session, command sequence, purpose, and recipient. Current shapes are:

`gameplay-command/{session_id}/{command_seq}/remote-target-notice`

`gameplay-command/{session_id}/{command_seq}/social/{purpose}/{recipient_character_id}`

`region-player-projection/{source_character_id}/{source_fence}/{version}/{recipient_character_id}/{recipient_fence}`

Disconnect-driven invite expiry has no gameplay command and uses its durable invite identity instead:

`{party|clan}-invite/{invite_id}/disconnect-expired/{recipient_character_id}`

One command may produce multiple immutable recipient events. Finalizing `gameplay_command_records`, inserting sanitized `chat_messages` history, and inserting every collected remote chat row happen in one PostgreSQL transaction. Local chat recipients are notified only after that commit. If chat, event validation, insertion, or command finalization fails, the whole transaction rolls back and the sender receives `system.persistence_failed` rather than a success projection. An identical command replay reads the stored outcome and does not run the producer or local fanout again. A conflicting replay remains `sequence.conflicting_replay`.

Command-driven party and clan operations now reuse that same PostgreSQL transaction for the authoritative repository mutation, final command outcome, and every collected outbox row. The pending dedup reservation is created before the transaction, so a pre-mutation crash can leave only a recoverable pending record; it cannot leave a social mutation without its applied outcome/events. Local socket fanout and state refresh run only after commit. The memory adapter serializes the same boundary and restores a social-state/outcome/outbox snapshot on failure.

Disconnect-driven invite expiry has no command record and remains an explicit safe fallback: deletion and event production are separate, while the immutable invite-derived idempotency key makes retry harmless. A crash after deletion and before production can lose that lifecycle notice; it cannot create membership, retain an invalid invite, duplicate a remote notice, or publish local success. Closing this non-command notice-loss window would require a dedicated expiry transaction/outbox producer in a later slice.

The selected target itself remains runtime-only and is not persisted. The durable row represents only the notification intent created by the rejected remote interaction.

## Claim And Delivery

Each server process has a unique worker id under its stable `server_instance_id`. The background dispatcher:

1. polls independently from the gameplay request pipeline
2. selects only undelivered, non-dead-letter events for its exact instance whose retry deadline is ready
3. uses a short transaction with `FOR UPDATE SKIP LOCKED`
4. writes a bounded claim lease and returns the claimed rows
5. revalidates the target's current durable ownership and matching ready local runtime
6. reserves one durable recipient receipt for the event
7. delivers the supported event
8. marks the receipt consumed and the row delivered only while the worker still owns both live claims

Concurrent workers cannot claim the same live row. An expired claim may be reclaimed.

PostgreSQL time owns claim, delivery, retry, and dead-letter deadlines so clock skew between backend instances cannot steal or retain a claim early.

Transport is at-least-once. `gameplay_event_receipts` persists `event_id`, recipient session/character, destination instance, claim lease, `delivered_at`, and `consumed_at`. A consumed receipt survives consumer restart: redelivery of the same event skips the socket and lets the outbox finish without a second visual message. Concurrent consumers serialize on the receipt row, so only one owns delivery. Failed ownership/socket validation releases an unconsumed reservation; retry and dead-letter never become local success.

Regional player projections also use one exact-recipient receipt per outbox row. Their source fence plus version provides a second ordering barrier: an older delivered event cannot overwrite or resurrect a newer projection even when transport order differs from production order. The outbox now also performs durable supersession per source/recipient route, so an older undelivered projection row becomes ineligible for claim as soon as a newer `(source_fence, version)` or despawn row for that route is persisted. See `cross-instance-region-player-projections.md`.

Every delivery also carries the stable monotonic `event_id`, and the browser read-model keeps a bounded set of remote social event ids. Party and clan state still changes only from the freshly rehydrated authoritative delta sent before the notice; a notice alone is never mutation authority.

The WebSocket and PostgreSQL cannot share one transaction. If the process dies after the socket accepts a payload but before `consumed_at` commits, the expired pending receipt may be redelivered. In-process suppression and the live browser event-id set cover ordinary retry, while a simultaneous server and page restart can still show a duplicate. Eliminating that final ambiguity requires a protocol-level client consume acknowledgement with durable server correlation; this slice does not claim transport-level exactly-once.

## First Event: Remote Target Notice

When `select_target` references a known player whose active owner is another instance:

- the actor still receives `presence.target_remote`
- the actor's local target does not change
- no PvP damage, party invite, clan invite, selection success, or remote runtime fallback occurs
- the command outcome and one `presence.remote_target_notice.v1` event commit atomically
- the target instance revalidates exact character and session ownership before emitting `presence_notice`

The notice contains only the outbox event id, notice type, actor character id, stable reason code, and server timestamps. It is lifecycle information, not snapshot or delta authority. The browser does not derive target, presence, or combat state from it.

Remote PvP remains unsupported and continues to return `presence.target_remote`.

## Remote Social Events

The social fanout contract currently supports three versioned event types:

- `social.chat_message.v1` for one server-owned, normalized, maximum-240-rune whisper or region message to one online remote character
- `social.party_notice.v1` for invite, accept, decline, leave, kick, dissolve-by-roster-rule, and related party lifecycle feedback
- `social.clan_notice.v1` for invite, accept, decline, leave, kick, and dissolve lifecycle feedback

Whisper lookup uses canonical persisted character identity and durable ownership. An offline or unknown recipient is rejected as `chat.whisper_target_not_found`; a remote recipient creates one exact-session event rather than local fallback.

Region chat requires the actor's authoritative non-empty region. The server lists active ownerships in that region, excludes the actor and current-instance recipients from the remote set, and creates one `chat-region` event per remote character/session. Ready local recipients are snapshotted from the local runtime but receive only after the history, command outcome, and complete remote event set commit. A recipient that is offline, unavailable, malformed, or outside the region does not invalidate delivery to otherwise valid recipients. Party chat remains local-instance in this slice.

Party and clan commands still resolve invite identity from the actor's authoritative current target. If a previously local known target moves to a remote owner before the invite command, the backend may create the durable invite and exact-owner notice after revalidating social eligibility. Accept, decline, leave, kick, and dissolve notify every currently online affected remote member with a distinct stable purpose/recipient key.

On consumption, ownership and the exact target instance, session, character, and fencing token captured at production are revalidated. Region-chat delivery additionally requires the event target region, current ownership region, and attached runtime region to match. Party/clan delivery loads current durable social state and emits an authoritative delta before the lifecycle notice. The payload cannot supply region scope, membership, roster, invite, or delivery success to the read-model.

### Ownership drift policy

Events do not reroute in this slice. If the recipient is offline, no longer owned by the destination instance, attached under a different target session, or attached under the same durable session id with a newer fencing token, delivery fails with stable internal code `social.recipient_offline` or `social.recipient_stale_owner`. Its unconsumed receipt reservation is released, the row follows normal retry/backoff, and it eventually dead-letters without a visual success. A previously consumed receipt wins over later ownership drift and suppresses redelivery. This deliberately avoids guessing a new destination or duplicating a message across consecutive ownership epochs.

## Retry, Dead Letter, And Retention

Delivery failures store a bounded machine-oriented error summary, increment `retry_count`, release the claim, and set an exponential retry deadline. After the configured maximum, the row receives `dead_lettered_at` and is no longer claimable. A failed event never blocks server startup or the gameplay command pipeline.

Retention deletes only rows whose `delivered_at` is older than the configured retention window. Pending, claimed, retrying, and dead-letter rows are never removed by delivered-event retention.

Superseded projection rows are compacted by a separate obsolete-row cleanup path instead of waiting for delivered retention. This keeps backlog pressure bounded without deleting the latest valid row for a stream and without treating stale projection backlog as delivery success.

Receipt rows follow their delivered outbox row through `ON DELETE CASCADE`; retention therefore removes only receipts whose successfully delivered parent is already old enough.

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
- `projection_row_superseded` and `compacted_obsolete` on the projection-specific metric family
- `expired` for retention deletion

Logs include event id, event type, destination instance, retry count, and a bounded failure code when applicable. They exclude payloads, attach tokens, access tokens, and account credentials.

`l2bg_social_fanout_events_total{category,result}` and structured `social_fanout` logs additionally classify `chat`, `party_notice`, and `clan_notice` as produced, delivered, duplicate, stale-owner, failed, or dead-letter. Message text and the JSON payload are never logged.

`l2bg_gameplay_event_receipts_total{category,result}` and structured `gameplay_event_receipt` logs cover `receipt_created`, `duplicate_receipt`, and `consumed`. Stale-owner and dead-letter remain classified by the social/outbox lifecycle metrics. Receipt logs contain routing ids but never whisper text or payload JSON.

`l2bg_region_chat_events_total{result}` and structured `region_chat` logs cover `region_chat_produced`, `local_delivered`, `remote_enqueued`, `remote_consumed`, `duplicate`, `stale_owner`, and `dead_letter`. They contain only routing/lifecycle metadata and never message text or payload JSON.

`l2bg_region_projection_events_total{result}` and structured `region_player_projection` logs cover production, consumption, duplicate/stale suppression, expiry, despawn, failure, and dead-letter without logging payload JSON, display name, position, or visual target.

Regional projection publication also exposes bounded queue/coalescing pressure counters and gauges. Delivery exposes event-age sum, count, and maximum gauges so a two-instance run can report average and maximum outbox delay without logging payloads. The canonical operational scenario is `docs/operations/multi-backend-fanout-validation.md`.

## Memory Adapter

The memory adapter shares the same semantics for deterministic tests:

- monotonic event ids
- immutable idempotency keys
- exact instance routing
- mutex-serialized claim leases
- retry and dead-letter state
- delivered-only retention
- atomic command outcome plus event creation under one lock
- durable receipt reservation/consume semantics shared by multiple `Store` wrappers
- rollback of party/clan mutation, command outcome, and outbox as one serialized test boundary
- active-ownership listing by region with the same lease/status filtering as PostgreSQL

Two `Store` wrappers may share one memory backend to simulate separate server instances without adding infrastructure.

## Non-Goals

- remote damage or cross-instance combat transactions
- cross-instance mob/NPC/loot/pet replication or any remote entity authority beyond player visual projection
- cross-instance party-chat broadcast
- state-changing remote party/clan authority beyond the already persisted domain command
- Redis, broker, external queue, or event sourcing
- admin panel or manual replay UI
- payload logging
- client-authored delivery success

## Invariants

- only the exact destination instance may claim an event
- claim concurrency does not duplicate live delivery
- delivered marking requires the current unexpired worker claim
- identical command replay cannot duplicate the event
- identical region-chat replay cannot duplicate local delivery, persisted history, remote event, durable receipt, or browser projection
- an older or identical regional player projection cannot overwrite, resurrect, or duplicate a newer entity projection
- conflicting command replay remains rejected
- a remote notice never changes the actor's target or enables remote gameplay
- a remote social notice never becomes party/clan state authority in the browser
- exact-session-and-fence ownership drift retries and dead-letters without local fallback or implicit reroute
- failed delivery cannot fail the gameplay server loop
- retention removes only old delivered rows
- PostgreSQL remains durable truth for outbox state; runtime memory is only the live dispatcher projection
