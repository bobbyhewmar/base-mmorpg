# Cross-Instance Region Player Projections

## Purpose

Freeze the first cross-instance player entity and movement fanout contract. The feature exists only to render remote-owned players in another instance that currently owns recipients in the same authoritative region. It does not transfer gameplay authority, replicate mobs, or enable remote combat, trade, pickup, party state, AI, or movement commands.

The implementation reuses durable session ownership, the PostgreSQL gameplay outbox, exact-recipient receipts, runtime known-set messages, and the browser's existing remote-player interpolation. It adds no Redis, broker, external queue, map, geodata, picking, spawn, or asset change.

## Conceptual Reference Translation

The studied reference reinforced four generic responsibilities: region entry introduces an entity to relevant observers, movement updates the observers' current visual state, a newly relevant observer needs a current snapshot rather than movement history, and exit removes the observer's remembered entity. This project implements those concepts with its own ownership fences, monotonic projection versions, exact-recipient outbox rows, receipts, heartbeat snapshots, despawn tombstones, and TTL. No reference code, packet, schema, asset, or proprietary identifier is used.

## Event Contract

`presence.region_player_projection.v1` carries one `upsert` or `despawn` for one source player to one exact remote recipient ownership. The outbox row targets the recipient's current:

- `server_instance_id`
- `session_id`
- `character_id`
- region
- recipient ownership `fencing_token`

The payload contains only projection/routing data:

- action
- source `character_id`, canonical display name, session, instance, and region
- authoritative position and facing
- whether movement is active and the accepted destination when present
- visual target id when present
- minimal canonical appearance required by the existing player renderer
- source ownership `fencing_token`
- source-local monotonic projection `version`

It excludes account/auth data, inventory, chat text, CP/HP/MP, combat counters, cooldowns, party/clan truth, and payload-authored authority.

The immutable idempotency key is:

`region-player-projection/{source_character}/{source_fence}/{version}/{recipient_character}/{recipient_fence}`

One source update may therefore create one row per eligible remote-owned recipient. Same-instance observers continue to use the direct runtime presence path.

## Production

The owner captures a projection snapshot when the player:

- activates after attach
- changes authoritative position or facing
- changes another visual projection field
- changes region after ownership renewal
- disconnects/unregisters
- remains stationary long enough for the two-second heartbeat

Publication is placed on a bounded in-process queue and persisted by a dedicated publisher, so PostgreSQL writes do not block command or movement application or the delivery dispatcher. When the primary queue is full, a bounded latest-per-source buffer coalesces superseded not-yet-persisted snapshots. When both bounds are exhausted, the new request is dropped with explicit pressure telemetry. A later heartbeat repairs a missed upsert and TTL repairs a missed despawn. Default heartbeat is two seconds and default projection TTL is six seconds.

The current eligibility contract is stricter than bare same-region ownership for `upsert`: the producer resolves active remote ownerships in the same region, then filters them by the recipient's current authoritative ownership anchor (`position_x`, `position_z`) against the source's authoritative projection path. When the source is moving, the path is the current position plus accepted movement destination; otherwise it is the current position only. New recipients must enter a slightly tighter interest radius than the configured retention radius, while already-relevant recipients may stay until they leave the full configured radius. This hysteresis reduces edge churn and avoids sending cold near-edge recipients that are not yet visually relevant enough.

The owner refreshes the ownership anchor from its live runtime on a short heartbeat cadence, without changing ownership authority or adding new infrastructure. It also keeps a per-source/fence runtime set of currently relevant exact-recipient routes. When a previously relevant recipient leaves the refined interest rule, the producer emits one exact-recipient `despawn` for that route instead of waiting only for TTL. `despawn` remains same-region-wide for explicit source disappearance or region transition, while TTL remains the safety net when a recipient simply stops receiving current state.

Once persisted, a newer projection row for the same `source_character_id + recipient_character_id + recipient_fence` route durably supersedes any older undelivered row with a lower `(source_fence, version)` pair. Superseded rows become ineligible for claim, clear any transient claim lease, and are compacted separately from normal delivered-event retention. A despawn therefore supersedes older upserts for that route, and a newer source fence supersedes every older fence for the same recipient epoch.

For a region transition, the producer advances the same source version sequence, emits a despawn for the previous region, then an upsert for the new authoritative region. The current world has no player-facing region-transfer command yet, but ownership renewal and the projection producer share this transition contract.

## Consumption and Ordering

The destination instance claims only its own outbox rows that are not already superseded and reserves the existing durable receipt before delivery. Consumption revalidates the exact recipient instance, session, character, captured fencing token, and runtime region. A reused durable session id under a newer fence is still ownership drift, not the original recipient. Drift remains `social.recipient_stale_owner`, with the existing retry/dead-letter policy and no automatic reroute.

An upsert also revalidates the source ownership tuple and region. If that owner expired or changed before delivery, the event is consumed as stale projection data and cannot make the player reappear. A despawn may arrive after source release; the destination accepts it subject to fence/version ordering. If a previously claimed row becomes durably superseded before socket delivery, the dispatcher skips delivery, records stale-delivery observability, and leaves later cleanup to compact that obsolete row without retry or dead-letter noise.

Each recipient runtime compares `(fencing_token, version)` lexicographically:

- a higher fence always supersedes a lower fence
- within one fence, only a higher version changes projection state
- an identical pair is a duplicate
- a lower pair is stale and cannot overwrite or resurrect newer state

Despawn and TTL retain an in-memory ordering tombstone for the attached runtime. Removing the visual entity therefore does not allow a delayed older upsert to resurrect it.

## Runtime and Known-Set

An accepted upsert creates or updates the existing `player_character` runtime entity with `projection_only: true`, projection fence/version, authoritative position/facing, movement presentation state, visual target, display identity, and minimal appearance. It is sent to the browser through the existing authoritative `entity_appear` or entity delta contracts.

The projected entity is visible and is present in that recipient runtime's volatile known-set, but it grants no local ownership. Every player command still resolves durable ownership at application time. In this slice:

- `select_target` returns `presence.target_remote`
- PvP returns the existing remote-presence rejection
- damage, pickup, trade, AI, and other gameplay mutations are not routed through the projection
- the browser does not create target or command success when the entity appears

`despawn` removes the entity with reason `remote_projection_despawn`. If no newer snapshot arrives within TTL, the runtime removes it with `remote_projection_expired`. Target cleanup follows the existing authoritative `entity_disappear` read-model path.

## Browser Projection

The browser consumes only server `entity_appear`, delta, and `entity_disappear`. The existing other-player projector interpolates authoritative position/facing updates over its bounded interpolation window and snaps only for extreme divergence. Movement destination is presentation metadata, not permission to simulate an authoritative path. A target command remains pending until backend outcome; appearance never sets local target or damage state.

## Observability

`l2bg_region_projection_events_total{result}` and structured `region_player_projection` logs cover:

- `projection_produced`
- `projection_consumed`
- `stale_ignored`
- `projection_row_superseded`
- `out_of_interest`
- `stale_delivery_skipped`
- `expired`
- `despawned`
- `compacted_obsolete`
- `duplicate`
- `failed`
- `dead_letter`

The publisher and delivery path additionally expose:

- `l2bg_region_projection_queue_events_total{result}` for enqueue, dequeue, coalescing, and bounded drop
- `l2bg_region_projection_fanout_total{result,reason}` for candidates before filtering, eligible after filtering, filtered-out reason codes, and produced rows
- queue depth, configured capacity, and coalesced-buffer depth gauges
- delivery delay sum, count, and maximum gauges derived from durable event creation time

Logs contain routing ids, fence, version, action, lifecycle result, and bounded reason code. They never include payload JSON, position, display name, target, auth data, or chat content.

The reproducible two-instance fault/load procedure and measured local baseline are documented in `docs/operations/multi-backend-fanout-validation.md`.

## Invariants

- only a fenced owner publishes a player projection
- only exact same-region remote ownerships receive a row, and `upsert` delivery is further reduced to recipients whose authoritative ownership anchor remains relevant to the source's current point-or-movement corridor under the enter/retain hysteresis rule
- exact-recipient receipts make transport redelivery safe across consumer restart
- exact-recipient validation includes the captured recipient fence; session-id reuse cannot cross takeover
- old or duplicate versions never duplicate an entity or overwrite newer visual state
- despawn and TTL remove stale visuals and preserve an ordering tombstone
- projected presence never becomes command, movement, PvP, trade, pickup, or AI authority
- no browser fallback creates remote success
- the memory adapter and PostgreSQL-backed stores share the same outbox/receipt semantics

## Explicitly Deferred

- remote PvP or damage transaction
- remote trade, pickup, party-state replication, or interaction
- mob, NPC, loot, pet, summon, or AI replication
- party-chat broadcast
- seamless region transfer UX
- Redis, broker, external queue, or entity event sourcing
