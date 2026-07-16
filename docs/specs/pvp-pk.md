# Authoritative PvP/PK Slice

## Purpose

Freeze the first backend-authoritative player combat contract without pulling siege, olympiad, clan war, alliance war, PvP events, rankings, or complex economic penalties into the slice.

## Concept Study Translation

The reference source was used only to extract responsibilities and lifecycle concepts. The useful concepts translated into this project's own model are:

- acquiring a player target and authorizing hostile damage are separate decisions
- membership in the current `known-set` is necessary but never sufficient for damage
- the backend validates actor state, target state, region policy, social relation, range, skill, resource, and cooldown at command application time
- a successful hostile action starts or refreshes a temporary PvP exposure deadline owned by the server
- that deadline is durable as an absolute timestamp, so reconnect or logical restart cannot erase a still-valid exposure and cannot resurrect an expired one
- peace/safe areas are a separate region policy from target selection, range, and damage calculation
- kill classification happens exactly once at death from the victim's pre-hit exposure state
- killing a PvP-exposed or karma-positive victim is a PvP kill; killing an unflagged, karma-neutral victim is a PK
- PK consequences and the absolute exposure deadline are durable; the runtime only projects whether that deadline is currently active
- death classification and consequences occur before the existing backend-owned respawn transition
- concurrent combat against either participant must serialize on persistent character authority rather than relying on a process-local mutex
- recent applied-hit history can be reused as a short durable attribution ledger, bounded by time and the victim's latest death boundary
- repeated killer/victim pairs are an investigation signal independent from gameplay blocking or rewards
- event, duel, siege, olympiad, war, summon attribution, rewards, economic drops, and advanced karma reduction are separate policy layers

No source code, schema, packet shape, asset, or proprietary identifier is part of this contract.

## Commands

The slice reuses the canonical commands rather than introducing a client-only PvP verb:

- `select_target` selects a known player and changes only authoritative target state
- `basic_attack` with a player `target_id` requests one immediate player attack
- `use_skill` with a player `target_id` requests one immediate single-target player skill

All three use the standard envelope, sequence handling, durable dedup record, `ack`/`reject`/`delta` lifecycle, and stored replay outcome. An identical replay cannot apply damage or consequences twice. A conflicting replay is rejected as `sequence.conflicting_replay`.

## Eligibility

A player attack is legal only when all of these are true at application time:

- actor and target are distinct attached characters
- actor owns the exact current durable session fence on this server instance
- target has an active ownership fence backed by a ready runtime on the same server instance
- actor and target are alive
- target is a known `player` in the actor's current authoritative `known-set`
- both characters are still attached to the same region
- the region enables open PvP and neither actor nor target is inside a configured safe area
- actor and target are not in the same authoritative party
- actor and target are not in the same authoritative clan
- target is within the command's authoritative range
- the skill is learned, active, supported for PvP, off cooldown, and affordable when `use_skill` is used

The current clean prototype regions `stonecross_plaza` and the compatibility test region `dawn_plaza` enable open PvP outside their minimum spawn sanctuary. The sanctuary is an application-owned policy rectangle (`x=-12..-4`, `z=-4..4`) evaluated from authoritative positions. It does not alter terrain, picking, geodata, bounds, spawn, checkpoints, renderer, or assets. Other regions fail closed with `pvp.region_restricted` until a policy is authored.

Attacking an otherwise eligible unflagged player is allowed. The risk is classified if that attack kills the target. There is no client-authored force-use switch.

## Damage and Death

- damage is computed from backend-owned attacker attack and defender defense
- player damage consumes CP before HP
- out-of-range player attacks reject immediately; this slice does not auto-approach or auto-repeat against players
- only `single_target_enemy` active skills are enabled against players in this slice
- target-centered AoE remains PvE-only and rejects against a player with `pvp.skill_not_supported`
- zero HP enters the existing backend-owned dead state, clears offensive target, queued or automatic attack, queued loot approach, active movement, temporary PvP flag, and cooldown state, then schedules the existing simple respawn
- respawn restores CP, HP, and MP at the existing checkpoint; the client does not choose timing, position, or restored resources
- PvP grants no XP, loot, currency, ranking reward, or economic drop in this slice

## PvP Flag, PK, and Karma

- a successful hostile player hit starts or refreshes a 30-second `pvp_flag`
- `pvp_flag_until` is an absolute durable deadline; reconnect and logical restart restore it only while it remains valid
- expiry is evaluated by server time, clears the durable deadline, and projects `pvp_flagged=false`, `pvp_flag_until_ms=null`, plus the state-transition reason `pvp.flag_expired`
- `pvp_kills`, `pk_count`, and `karma` are durable non-negative character fields
- killing a victim whose PvP flag is active or whose karma is positive increments `pvp_kills`
- killing an unflagged victim with zero karma increments `pk_count` and adds 100 karma
- this slice has no karma decay, karma-reduction quest, item drop penalty, XP loss, jail, bounty, ranking, or economy penalty

The fixed duration and fixed karma increment are project-owned minimum constants. They are not compatibility promises for a future balance pass.

## Persistence and Fanout

Before emitting a successful player-combat delta, the backend opens one PostgreSQL transaction, locks the attacker and victim character rows in deterministic character-id order, and reloads their durable CP, HP, MP, flag, PvP/PK, and karma state. Damage, MP payment, attacker cooldown start, both combat resource states, both exposure deadlines, death classification, PvP/PK/karma counters, lethal victim cooldown cleanup, attribution, anti-feed signal, and one `pvp_combat_events` row are then committed as one unit. The PostgreSQL transaction clock owns the deadline and audit timestamp so application-instance clock skew does not decide ordering.

The process-local PvP mutex remains only as a local runtime coordination optimization. It is not the multi-instance correctness boundary. The memory adapter uses one critical section and the same mutation resolver, cooldown checks, attribution rules, and anti-feed rules as PostgreSQL.

Session fencing precedes player combat. The actor lease is renewed before command dedup or `ack`; the target's exact local fence is renewed before the combat transaction. A known target owned by another instance rejects as `presence.target_remote`, because this slice deliberately has no remote combat router or fallback.

The handler projects the committed state back into the two locked runtime actors and performs volatile death cleanup before publishing success. The generic post-command progression/cooldown flush is skipped for player combat, preventing a stale runtime snapshot from overwriting the newer locked transaction. Persistence failure returns `system.persistence_failed` without publishing local or runtime success.

Durable command reservation still precedes domain application. The audit table additionally enforces at most one event for a non-empty `session_id + command_seq`, so an identical replay cannot duplicate damage or audit even across process boundaries; a conflicting replay remains `sequence.conflicting_replay`.

Each audit row records attacker and victim character/account identity, action and optional skill, applied CP and HP damage, hit/PvP-kill/PK-kill result, exposure state before and after, PvP-kill/PK-count before and after, karma before/after/delta, timestamp, command metadata, primary killer, assist character ids, suspicious state, and repeated-kill count. `GET /internal/pvp/events` is read-only, paginated, disabled by default, and protected by the same `X-Internal-Audit-Token` contract used by the existing internal audit surface. It supports attacker, victim, involved character, killer, suspicious, action/action-type, result, time-window, limit, and offset filters.

The actor receives a correlated delta with their authoritative resources, cooldown, target, flag, counters, and a target entity patch. The victim receives their self delta through the authoritative runtime tick, and other sessions receive the updated player presence. The browser only projects these snapshots and deltas.

## Attribution and Anti-Feed Signal

- every applied player hit remains a durable attribution candidate for 30 seconds
- on a lethal hit, the current attacker is the primary killer
- assists are distinct prior attackers who applied positive damage to that victim inside the window, ordered by most recent hit and excluding the killer
- attribution never crosses the victim's previous `pvp_kill` or `pk_kill` boundary, so hits from a previous life cannot assist a later death
- on every kill, the backend counts prior kills by the same killer/victim pair during the preceding 10 minutes
- `repeated_kill_count` includes the current kill; the first is `1`, and the second or later is marked `suspicious=true`
- suspicious events do not reject commands, change damage, change classification, or grant/remove rewards in this slice
- no client field can nominate killer, assists, repeated count, or suspicious state

These windows are project-owned investigation constants, not balance or reward promises.

## Stable Reason Codes

| Reason code | Meaning |
| --- | --- |
| `pvp.self_target` | Actor attempted to attack their own character |
| `pvp.target_unavailable` | Previously known player is no longer attached |
| `presence.target_remote` | Known player is online under another server instance and cannot be attacked through the local runtime |
| `pvp.target_out_of_region` | Attached target is no longer in the actor's authoritative region |
| `pvp.region_restricted` | Current region does not enable open PvP |
| `pvp.safe_zone` | Actor or target is inside a server-authored safe area |
| `pvp.flag_expired` | Server-owned exposure deadline expired and was cleared; this annotates a state delta rather than rejecting an attack |
| `pvp.same_party` | Actor and target share the same authoritative party |
| `pvp.same_clan` | Actor and target share the same authoritative clan |
| `pvp.skill_not_supported` | Skill category is outside the current single-target PvP slice |
| `world.entity_not_known` | Target is absent from the current authoritative known-set |
| `combat.actor_dead` | Actor is dead |
| `combat.target_dead` | Target is dead |
| `combat.out_of_range` | Target is outside the authoritative attack or skill range |
| `combat.cooldown_active` | Attack or skill cooldown is active |
| `combat.insufficient_mp` | Actor cannot pay the skill MP cost |
| `system.persistence_failed` | Durable two-character combat state could not be committed |

## Explicitly Deferred

- richer named-zone/content volumes beyond the minimum server-only spawn sanctuary policy
- AoE or chain PvP
- automatic player chase and repeated auto-attack
- pets, summons, contribution weighting, party-based attribution, and non-player damage-source attribution
- clan war, alliance war, duel, siege, olympiad, events, and competitive matchmaking
- karma decay and complex economic or death penalties
- PvP rewards, leaderboards, anti-feed blocking/scoring, account/device correlation, alert automation, and complete anti-grief enforcement
