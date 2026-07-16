# Session Ownership and Cross-Instance Presence

## Purpose

Freeze the minimum durable ownership and fencing contract that lets more than one backend instance share PostgreSQL without allowing two instances to author the same online character.

The ownership slice adds no Redis, external queue, shard transfer, map change, or client fallback. The later PostgreSQL outbox slice now builds one minimum informational fanout path on this ownership registry; its separate contract is `postgres-gameplay-event-outbox.md`.

## Concept Translation

The source reference contributed only lifecycle concepts: one active character binding, explicit connection states, old-binding invalidation on replacement, detach before cleanup, and idempotent disconnect. The project implements those concepts independently with a database lease and fence. No source code, identifiers, packets, schemas, or assets are part of this contract.

## Durable Ownership Record

`gameplay_session_ownerships` contains exactly one current ownership row per character:

- `character_id`
- `session_id`
- `server_instance_id`
- monotonic positive `fencing_token`
- current `region_id`
- absolute `lease_expires_at`
- `acquired_at`
- `renewed_at`

`gameplay_sessions` remains the durable session and attach-credential record. The ownership row is the online authority record; status `attached` alone is not proof of a live owner.

PostgreSQL time owns acquire and renewal deadlines. The memory adapter uses the same acquire, conflict, renewal, expiry, replacement, and conditional-release semantics under one critical section.

## Acquire and Attach

Attach validates the session, attach token, token deadline, and allowed session status inside the ownership acquisition boundary. PostgreSQL locks the character row before inspecting or changing ownership, serializing concurrent attach attempts even when they arrive through different pools or server processes.

Rules:

- no ownership row creates fence `1`
- an unexpired ownership lease cannot be replaced by a different gameplay session
- a reconnect for the same active session on its current server instance may replace the owner immediately, but only with the current attach credential; local command dispatch is drained first, then acquisition rotates that credential atomically and increments the fence
- a different server instance cannot replace an unexpired owner, even for the same session; cross-instance takeover requires release or lease expiry
- concurrent attaches using the same credential therefore produce exactly one durable winner; later contenders receive stable `session.invalid_attach_token`, while a different session contending with a live owner receives `session.ownership_conflict`
- an expired ownership may be replaced; the fence increments and a different previous session becomes `closed`
- only the acquired session is changed to `attached`
- `world/enter` may return the current active session and current rolling attach credential to the authenticated account; a successful reconnect rotates the credential, advances the fence, and makes the previous socket stale before the new binding is activated
- startup sanitation closes only `attached` sessions without an unexpired ownership row; starting another instance must not close a live owner

The defaults are a 30-second lease, renewal every 10 seconds, and a rolling 5-minute attach-token deadline. Attach credentials are never logged and are rotated on every successful acquisition or reconnect. These values are deployment configuration, not gameplay balance.

## Renewal, Commands, and Fencing

Every production gameplay command renews and validates the exact tuple before dedup lookup or `ack`:

`character_id + session_id + server_instance_id + fencing_token`

If the tuple is no longer current or the lease expired, the server returns the early reject `session.stale_owner`. The command does not advance `command_seq`, reserve a dedup record, apply runtime state, or publish success.

The WebSocket tick renews idle ownership periodically. Async movement revalidates the fence again before applying its delayed result. A different instance cannot replace an unexpired lease, and same-instance reconnect drains the old socket's serialized command dispatch before advancing the fence. Command validation therefore extends a bounded exclusion window before application. Future work that deliberately runs longer than the lease must carry the fence into its final durable commit rather than relying on this bounded command contract.

## Detach, Release, and Replacement

Release expires ownership as a durable tombstone and closes the session only when the exact owner tuple still matches. The next acquisition increments the preserved fence rather than resetting it. Therefore:

- double unregister or double release is a no-op after the first cleanup
- an old socket cannot release a newer owner
- an old socket cannot close a reused session after a higher fence exists
- stale disconnect cleanup does not cancel current party or clan invites
- final character checkpointing is attempted only after the old owner successfully renews its exact fence

When a local attachment is replaced after reconnect or expiry, the local registry removes the older character binding before activating the new one. Gauge accounting remains balanced and peers reconcile the replacement through authoritative presence state.

## Minimal Cross-Instance Presence

`known-set` remains runtime-only and is never persisted. The ownership registry provides a separate minimum presence classification for a referenced character:

- `local`: active ownership belongs to this instance and a matching ready runtime is registered
- `remote`: an unexpired ownership belongs to another `server_instance_id`
- `offline`: no unexpired ownership exists
- `unavailable`: ownership claims this instance but the matching ready runtime is not available yet

This classification does not synthesize a remote runtime entity. It prevents remote-online characters from collapsing into fake local success or the same meaning as unknown/offline, and it may provide the exact destination ownership used by the durable outbox.

For a player already present in runtime `known-set`, `select_target`, player PvP, party invite, and clan invite return `presence.target_remote` when that player is currently owned by another instance. A previously known player with no active ownership returns the existing domain-unavailable behavior or `presence.target_offline` for selection. A character absent from `known-set` still returns `world.entity_not_known`.

`select_target` now also produces one replay-safe informational remote-target notice for the destination owner. The notice does not select the target, authorize damage, create an invite, or replace `presence.target_remote`. PvP, party, clan, movement, entity, and chat interaction remain local-only until dedicated cross-instance contracts exist.

## Stable Reason Codes

| Reason code | Meaning |
| --- | --- |
| `session.invalid_attach_token` | Attach used a consumed, rotated, unknown, or otherwise invalid credential |
| `session.ownership_conflict` | A different gameplay session attempted to replace an unexpired durable owner |
| `session.stale_owner` | Gameplay command came from an expired or superseded owner tuple |
| `presence.target_remote` | Known player is online on another instance; the command remains unsupported even if an informational notice is queued |
| `presence.target_offline` | Previously known player has no active durable ownership |

These are backend decisions. The browser only renders rejects and authoritative snapshots or deltas.

## Observability

Structured `session_ownership` events and `l2bg_session_ownership_events_total{result=...}` cover:

- acquired
- renewed
- replaced after expiry
- ownership conflict
- expired
- rejected stale owner
- released

Logs include session, character, instance, fence, and lease deadline when available, but never the attach token.

## Explicitly Deferred

- cross-instance entity, movement, combat, party, clan, or chat fanout beyond the informational remote-target notice
- remote party/clan invite delivery
- remote PvP execution
- Redis, queue, or a new coordination service
- region transfer fencing beyond updating the ownership region during renewal
- long-running gameplay jobs that exceed the lease
- seamless shards, siege, olympiad, clan war, and PvP events

## Invariants

- one character has at most one unexpired durable owner
- only the exact current owner tuple accepts gameplay commands
- fence values never decrease for a character ownership row
- startup of one instance does not invalidate another instance's live lease
- release and unregister are idempotent and conditional
- `known-set` remains volatile; ownership is durable presence classification, not durable visibility
- remote-online never falls back to a local gameplay mutation
