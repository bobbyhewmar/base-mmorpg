# Command Lifecycle

## Objective

Define the official lifecycle for authoritative gameplay commands in the first online slice.

This document freezes:

- the processing pipeline
- the meaning of `ack`
- the meaning of `reject`
- the meaning of `delta`
- the distinction between early failure and domain failure
- the namespace strategy for `reason_code`

## Decision

The official lifecycle is:

1. `command received`
2. `pre-validation`
3. `ack` or `early reject`
4. `domain validation`
5. `apply`
6. `commit` or runtime state update as applicable
7. authoritative outbound broadcast

The lifecycle does not define a separate `applied` message type for the initial slice.

Successful application is represented by an authoritative outbound message such as `delta`, `trade_notice`, `party_notice`, or `chat_message`.

## Official Pipeline

### 1. Command Received

The server receives a gameplay command envelope over the gameplay transport.

At this point, the command is not yet accepted for processing.

Party affordances such as `/invite` and `/leave` are normalized by the browser into gameplay command envelopes before this stage. `send_chat_message` is not the transport entry point for party mutations.

### 2. Pre-Validation

The server performs transport and session checks:

- envelope can be parsed
- `protocol_version` is supported
- connection is authenticated
- gameplay session is attached
- the durable ownership tuple for the actor still matches `session_id + character_id + server_instance_id + fencing_token`
- the ownership lease is still active and can be renewed by this instance
- `command_seq` is syntactically valid
- duplicate handling can be resolved
- conflicting replay is not present

No gameplay rule is applied during pre-validation.

Ownership validation runs before dedup lookup or reservation. A stale instance cannot replay an earlier success after losing the fence and cannot reserve a new command record.

### 3. `ack` or Early `reject`

If pre-validation succeeds, the server sends `ack`.

If pre-validation fails, the server sends `reject` without sending `ack`.

For `move_intent`, the client may already be showing reversible predicted movement before `ack` arrives. `ack` is still not gameplay success; it only confirms that the command entered the authoritative pipeline.

### 4. Domain Validation

The server loads the authoritative runtime context and validates gameplay legality.

Examples:

- target exists in `known-set`
- `select_target` may select a known `player` for social interaction and HUD projection, but that selection alone never authorizes player damage or enables PvP/PK
- `basic_attack` or `use_skill` against a known player enters the server-owned PvP eligibility path; the browser cannot turn selection, ack, animation, or cooldown presentation into damage
- player combat revalidates live attachment, same-region membership, safe-area/region policy, party/clan relation, life state, range, skill, MP, and cooldown before any mutation
- a successful player hit locks both durable character rows in deterministic order and atomically checkpoints combat resources, exposure deadlines, attacker cooldown, lethal victim cleanup, PvP/PK consequences, attribution/anti-feed fields, and one combat audit event before emitting the correlated actor delta
- target is in range
- movement destination can be resolved through server-owned terrain/geodata
- movement route does not cross static blockers
- session character is alive if required
- skill cooldown is not active
- MP cost can be paid
- loot is present in the known-set and is either immediately reachable or can be approached through a server-selected pickup-range destination
- NPC interaction range and action legality are valid for the current authoritative quest or service state
- item is equipable
- consumable item is owned, still in inventory, and currently usable
- tame target is known, alive, tameable, and within authoritative tame range
- summon or dismiss is legal for the current owned companion state
- mount or dismount is legal for the current owned companion state
- party invite target is the actor's current runtime player target, is present in the authoritative `known-set`, has a ready local or durable remote owner, and is currently eligible
- party invite target is not the actor itself and is not already in a party
- the invitee does not already have a live pending invite and the inviter or party does not already have a live outbound invite
- only the current party leader may invite or kick
- party size does not exceed the canonical cap of 9 members
- party accept or decline references a live invite assigned to the actor
- inviter and invitee still satisfy the invite rules at accept time
- inviter or invitee disconnect cancels the pending invite instead of allowing stale acceptance
- clan invite target is the actor's current authoritative runtime player target; `invite_clan_member` accepts only an empty payload and rejects a client-authored `target_character_id` as `protocol.invalid_envelope`
- clan accept or decline references a live invite assigned to the actor, and accept revalidates membership plus expiry at commit time
- chat channel is supported for the current slice
- chat text is non-empty, within bounds, and not over the current rate limit
- `region` chat has a non-empty authoritative current region and recipients are server-resolved from local runtime plus active durable ownership in that same region
- actor death state does not block the current social chat slice
- `party` chat requires current authoritative party membership
- `whisper` requires an online target resolved by authoritative persisted character-name lookup and current ownership
- shared kill XP eligibility is resolved only from the current authoritative party, attach, region, and alive state at kill time
- party-owned loot pickup requires the actor to remain inside the loot's eligible party subset
- stack quantity is valid for split
- source and target stacks are merge-compatible
- hotbar slots have valid range, entry type, open-bar count, active learned skill bindings, owned item bindings, and supported action bindings

For movement, geodata/pathfinding must be bounded so one route calculation does not make the player feel frozen or block unrelated runtime work.

For loot pickup, the client sends exactly one `pick_up_loot` command. If the actor is outside immediate pickup range, the server may apply a movement delta with `movement_reason: "loot_approach"` and complete the inventory mutation later from the authoritative tick loop. The browser must not run a local retry loop or convert projected visual distance into a second command.

### 5. Apply

If domain validation succeeds, the server applies the command to authoritative runtime state and durable state as required.

### 6. Commit or Runtime Update

If the command changes durable state, the server completes the appropriate persistence boundary.

If the command changes runtime-only state, the server commits the runtime mutation in memory without forcing a durable write.

For movement, the authoritative runtime update may arrive after the client has already started predicted locomotion. The server outcome remains authoritative and the client must reconcile to it.

For queued gameplay interactions such as loot pickup after approach, the durable mutation happens only when the authoritative runtime reaches legal range. Intermediate approach waypoints are runtime state, not inventory success.

For a remote-owned known player, `select_target` still commits a rejected command outcome. That outcome and its optional `presence.remote_target_notice.v1` outbox row are finalized in one PostgreSQL transaction. The event idempotency key derives from `session_id + command_seq`; identical replay reads the stored command outcome without producing again, and conflicting replay remains `sequence.conflicting_replay`. The informational notice never changes target state and is not a substitute for `delta` or snapshot authority.

Remote social delivery uses the same command collector and may finalize more than one recipient event. Each key derives from `session_id + command_seq + purpose + recipient_character_id`. The command outcome, sanitized chat history, and complete remote event set commit together for whisper and region chat; failure replaces sender success with `system.persistence_failed`. Region-chat local fanout also waits for commit and revalidates the recipient's local region before socket delivery. Command-driven party/clan mutation, final command outcome, and all resulting remote notices share one transaction. The earlier pending dedup reservation may survive a pre-mutation crash, but an applied mutation cannot commit without its outcome/outbox.

If the command was accepted into the pipeline with a dedup reservation, the server finalizes the command record with:

- stable status
- command identity
- serialized outbound outcome for deterministic replay

For economy mutations such as vendor, exchange, warehouse, and player-trade flows, the same authoritative commit boundary is also responsible for persisting any required audit rows. Those rows must keep stable action types plus `session_id`, `command_id`, and `command_seq` when that metadata is available from the command context.

For pet mutations such as `tame_mob`, `summon_pet`, `dismiss_pet`, `mount_pet`, and `dismount_pet`, the same authoritative commit boundary is responsible for persisting ownership or state transitions in `character_pets`. Mounted move speed is derived from that persisted truth rather than from client presentation state.

For party mutations such as `invite_party_member`, `accept_party_invite`, `decline_party_invite`, `leave_party`, and `kick_party_member`, the same authoritative commit boundary is responsible for persisting leader, membership, or invite state in `parties`, `party_members`, and `party_invites`. Pending invite state stays ephemeral, the functional party is born or grows only on accept, and party roster or invite UI remains a projection of that backend truth.

For canonical-minimum party rules, the same authoritative boundary is also responsible for:

- expiring invites after 10 seconds
- canceling pending invites when inviter or invitee disconnects
- dissolving the party when leave or kick reduces the roster to one member
- transferring leadership deterministically to the oldest remaining member when the leader leaves and two or more members remain

For clan mutations such as `create_clan`, `invite_clan_member`, `accept_clan_invite`, `decline_clan_invite`, `leave_clan`, `kick_clan_member`, and `dissolve_clan`, the same authoritative commit boundary is responsible for persisting leader, membership, or invite state in `clans`, `clan_members`, and `clan_invites`. Clan UI remains a projection of backend truth and does not invent membership or invite success locally.

`accept_clan_invite` adds membership and consumes the invite in one repository transaction or one memory-store critical section. A failed validation or persistence step must leave both membership and invite state unchanged. Storage also enforces at most one live outbound invite per clan and one live inbound invite per invitee, in addition to runtime validation.

For canonical-minimum clan rules, the same authoritative boundary is also responsible for:

- trimming, validating, and globally de-duplicating clan names
- expiring invites after 10 seconds
- canceling pending invites when inviter or invitee disconnects
- enforcing current-target invite semantics plus leader-only invite, kick, and dissolve
- preventing the leader from using `leave_clan` in this phase
- keeping the clan valid at one member until explicit `dissolve_clan`
- leaving manual leader transfer and automatic leader transfer out of scope

For `send_chat_message`, the same authoritative commit boundary is responsible for persisting minimum chat history in `chat_messages`. Delivery scope remains server truth derived from current region, party membership, canonical character identity, and durable ownership. A remote whisper stores chat history, command outcome, and one `social.chat_message.v1` delivery intent atomically. Region chat stores the same history/outcome plus one exact-owner event per remote recipient and delivers to still-eligible local recipients only after commit. The current slice exposes only `region`, `party`, and `whisper`; party fanout remains local-instance. `local` remains reserved for a later distinct scope. The client never authors the final recipient set.

For player combat, a process-local mutex may coordinate runtime projection but cannot be the correctness boundary. PostgreSQL-backed mode serializes attacker/victim mutations through deterministic row locks and computes damage, death classification, counters, deadlines, cooldown mutation, attribution, repeated-pair signal, and audit from the locked durable state. The memory adapter mirrors this in one critical section. The generic post-command progression/cooldown flush must not run after this transaction, because it could overwrite a newer multi-instance combat state.

For shared party rewards, the same authoritative kill boundary is responsible for:

- resolving the eligible party subset from live attached runtime truth
- splitting kill XP deterministically across that subset
- marking spawned loot with minimum party ownership metadata when more than one member is eligible
- keeping the existing persistent pickup boundary in `character_items`

Round-robin, master loot, dice, redistribution UI, and broader reward orchestration remain outside this slice.

### 7. Authoritative Outbound Broadcast

The server emits authoritative outbound messages to the issuing client and to any other affected sessions as required by presence scope.

For state mutation flows, that outbound is usually `delta`.

For scoped social flows, the outbound may instead be a typed notice or message such as `trade_notice`, `party_notice`, `clan_notice`, or `chat_message`.

When a social recipient is remotely owned, the server writes `social.chat_message.v1`, `social.party_notice.v1`, or `social.clan_notice.v1` to the exact destination instance/session. The dispatcher reserves a durable receipt, revalidates ownership, and records consumption after socket acceptance. Party/clan consumers rehydrate current durable state and emit its delta before the notice. A consumed receipt suppresses redelivery after consumer restart; every remote social message also carries the monotonic outbox `event_id` for live runtime/browser duplicate suppression. Ownership drift does not reroute: an unconsumed reservation is released and the event retries or dead-letters with `social.recipient_offline` or `social.recipient_stale_owner`.

Every successful clan mutation sends the issuing client a `delta` carrying `applies_to_command_id` and `applies_to_command_seq`. An `ack` or uncorrelated `clan_notice` may provide lifecycle feedback, but cannot mark the command applied or mutate projected clan truth.

## Message Semantics

### `ack`

`ack` means:

- the server accepted the command into the authoritative processing pipeline for the active session

`ack` does not mean:

- the command is legal
- the command was applied
- state mutation was committed
- the outcome succeeded
- movement pathfinding has completed

Example:

```json
{
  "kind": "ack",
  "command_id": "01JZSKILL0001",
  "command_seq": 10,
  "status": "received"
}
```

### `reject`

`reject` means:

- the command did not produce an authoritative state mutation

`reject` may happen:

- before `ack`, as an early reject
- after `ack`, as a domain reject

Example:

```json
{
  "kind": "reject",
  "command_id": "01JZSKILL0001",
  "command_seq": 10,
  "reason_code": "combat.out_of_range",
  "message": "Target is out of range."
}
```

### `delta`

`delta` means:

- the authoritative state was updated or confirmed in a way the client must reconcile against

`delta` is the success-path outcome message.

### `chat_message`

`chat_message` means:

- the server accepted and scoped a chat message authoritatively for the current slice

`chat_message` does not mean:

- the client chose its own recipients
- offline delivery exists
- HTML or rich text is allowed

Example:

```json
{
  "kind": "delta",
  "revision": 1042,
  "applies_to_command_id": "01JZSKILL0001",
  "applies_to_command_seq": 10,
  "self": {
    "mp": 44,
    "cooldowns": {
      "crescent_strike": 900
    }
  },
  "entities": [
    {
      "entity_id": "mob_1",
      "hp": 21
    }
  ]
}
```

## Early `reject` vs `ack` Followed by `reject`

### Early `reject`

Early `reject` happens before the server has accepted the command into the authoritative gameplay pipeline.

Use early `reject` for:

- `protocol.invalid_envelope`
- `protocol.unsupported_version`
- `auth.not_authenticated`
- `session.not_attached`
- `session.stale_owner`
- `sequence.invalid`
- `sequence.conflicting_replay`

When a prior `session_id + command_seq` already exists with the same `command_id`, the server must replay the stored outcome instead of issuing a fresh `ack`.

### `ack` Followed by `reject`

`ack` followed by `reject` happens when the envelope and session are valid but gameplay legality fails.

Use `ack` followed by `reject` for:

- `world.entity_not_known`
- `world.entity_not_interactable`
- `world.loot_out_of_reach`
- `combat.out_of_range`
- `combat.cooldown_active`
- `combat.insufficient_mp`
- `inventory.item_not_found`
- `inventory.item_not_equippable`
- `inventory.item_not_usable`
- `inventory.item_not_stackable`
- `inventory.split_invalid_quantity`
- `inventory.merge_invalid`
- `npc.interaction_out_of_range`
- `npc.action_not_supported`
- `quest.action_unavailable`
- `system.persistence_failed`

## Reason Code Namespaces

Reason codes must use stable namespaces.

The initial namespaces are:

- `protocol.*`
- `auth.*`
- `session.*`
- `sequence.*`
- `world.*`
- `presence.*`
- `combat.*`
- `inventory.*`
- `npc.*`
- `quest.*`
- `movement.*`
- `pet.*`
- `mount.*`
- `chat.*`
- `party.*`
- `pvp.*`
- `system.*`

### Initial Reason Codes

| Reason Code | Meaning |
| --- | --- |
| `protocol.invalid_envelope` | Envelope shape or required field set is invalid |
| `protocol.unsupported_version` | `protocol_version` is not supported |
| `auth.not_authenticated` | Connection lacks authenticated user context |
| `session.not_attached` | No active gameplay session is attached to the connection |
| `session.invalid_attach_token` | WebSocket attach used a consumed, rotated, unknown, or expired credential |
| `session.ownership_conflict` | A different gameplay session attempted to replace another unexpired durable owner |
| `session.stale_owner` | Command came from an expired or superseded session ownership fence |
| `sequence.invalid` | `command_seq` is malformed or unusable |
| `sequence.out_of_order` | `command_seq` does not match the expected progression |
| `sequence.conflicting_replay` | Same `command_seq` was reused with conflicting command identity |
| `world.entity_not_known` | Referenced entity is not in the current `known-set` |
| `world.entity_not_interactable` | Referenced entity exists but cannot be interacted with |
| `presence.target_remote` | Known player is online on another server instance; the requested interaction remains unsupported even if an informational notice is queued |
| `presence.target_offline` | Previously known player no longer has active durable ownership |
| `world.loot_out_of_reach` | A queued loot pickup finished movement but still could not reach the loot |
| `combat.out_of_range` | Target is not within valid range |
| `combat.cooldown_active` | Skill is still on cooldown |
| `combat.insufficient_mp` | MP is insufficient for the action |
| `pvp.self_target` | Actor attempted to attack their own character |
| `pvp.target_unavailable` | Known player is no longer attached |
| `pvp.target_out_of_region` | Attached target is no longer in the actor's authoritative region |
| `pvp.region_restricted` | Current region does not enable open PvP |
| `pvp.safe_zone` | Actor or target is inside a server-authored safe area |
| `pvp.flag_expired` | Server-owned exposure deadline expired and was cleared; emitted as delta annotation, not command rejection |
| `pvp.same_party` | Actor and target share the same authoritative party |
| `pvp.same_clan` | Actor and target share the same authoritative clan |
| `pvp.skill_not_supported` | Skill is outside the current single-target PvP slice |
| `inventory.item_not_found` | Item instance cannot be resolved for the actor |
| `inventory.item_not_equippable` | Item cannot occupy the requested slot |
| `inventory.item_not_usable` | Item cannot be consumed or has no effect in the current authoritative state |
| `inventory.item_not_stackable` | Item cannot participate in stack operations |
| `inventory.split_invalid_quantity` | Split quantity is zero, negative, or consumes the full stack |
| `inventory.merge_invalid` | Source and target stacks cannot be merged into one authoritative stack |
| `npc.interaction_out_of_range` | NPC interaction range validation failed |
| `npc.action_not_supported` | The selected NPC action is not supported by that authoritative NPC service |
| `quest.action_unavailable` | The requested quest interaction is not valid for the current authoritative quest step |
| `movement.destination_blocked` | Requested movement destination is blocked by terrain or obstacle data |
| `movement.destination_out_of_bounds` | Requested movement destination is outside the authoritative region bounds |
| `movement.path_unreachable` | No legal route exists from the current position to the requested destination |
| `movement.path_budget_exceeded` | Pathfinding exceeded a configured safe budget |
| `movement.geodata_unavailable` | Authoritative geodata was unavailable for the current region |
| `movement.geodata_mismatch` | Movement reconciliation detected incompatible geodata version context |
| `pet.target_not_tameable` | Referenced target cannot become a companion in the current slice |
| `pet.tame_out_of_range` | Referenced tame target is outside authoritative tame range |
| `pet.ownership_limit_reached` | The current slice companion ownership cap has already been reached |
| `pet.not_owned` | Character does not own the required companion |
| `pet.already_summoned` | Owned companion is already summoned |
| `pet.not_summoned` | Owned companion is not currently summoned |
| `mount.not_mountable` | Owned companion cannot be mounted in the current slice |
| `mount.pet_not_ready` | Companion must be summoned before mounting |
| `mount.already_mounted` | Character is already mounted |
| `mount.not_mounted` | Character is not currently mounted |
| `mount.dismount_required` | Companion must be dismounted before dismiss is legal |
| `chat.channel_unknown` | Referenced chat channel is not supported in the current slice |
| `chat.region_unavailable` | Region chat sender has no authoritative current region |
| `chat.message_empty` | Chat text is empty after authoritative normalization |
| `chat.message_too_long` | Chat text exceeds the current maximum size |
| `chat.rate_limited` | Chat sender exceeded the current burst limit |
| `chat.party_required` | Party chat requires current authoritative party membership |
| `chat.whisper_target_required` | Whisper requires a target character name |
| `chat.whisper_target_not_found` | Whisper target is not currently online or resolvable |
| `loot.party_ineligible` | Referenced loot is reserved for a different eligible party subset |
| `party.target_not_known` | Referenced player is not currently in the authoritative known-set |
| `party.target_not_online` | Referenced player is not currently attachable for the current slice invite rules |
| `party.target_already_in_party` | Referenced player already belongs to a party |
| `party.target_invalid` | Referenced player is not a valid invite target for the command |
| `party.invite_already_pending` | Referenced player already has a live pending invite |
| `party.party_full` | The current party is already at the canonical 9-member cap |
| `party.invite_not_found` | Referenced invite no longer exists |
| `party.invite_not_recipient` | Referenced invite is not assigned to the acting character |
| `party.invite_expired` | Referenced invite has expired |
| `party.leader_required` | Only the current party leader may perform this mutation |
| `party.not_in_party` | Character is not currently in a party |
| `party.already_in_party` | Character already belongs to a party |
| `party.member_not_found` | Referenced party member is not currently present in the roster |
| `party.cannot_kick_self` | Party leaders cannot use kick against themselves |
| `clan.name_invalid` | Clan name failed authoritative trim, bounds, or regex validation |
| `clan.name_taken` | Clan name is already reserved by another clan |
| `clan.target_not_known` | Referenced player is not currently in the authoritative known-set |
| `clan.target_not_online` | Referenced player is not currently attachable for the current clan invite rules |
| `clan.target_already_in_clan` | Referenced player already belongs to a clan |
| `clan.target_invalid` | Referenced player is not a valid clan invite target for the command |
| `clan.invite_already_pending` | Referenced player already has a live pending clan invite or the clan already has a live outbound invite |
| `clan.invite_not_found` | Referenced clan invite no longer exists |
| `clan.invite_not_recipient` | Referenced clan invite is not assigned to the acting character |
| `clan.invite_expired` | Referenced clan invite has expired |
| `clan.leader_required` | Only the current clan leader may perform this mutation |
| `clan.not_in_clan` | Character is not currently in a clan |
| `clan.already_in_clan` | Character already belongs to a clan |
| `clan.member_not_found` | Referenced clan member is not currently present in the roster |
| `clan.cannot_kick_self` | Clan leaders cannot use kick against themselves |
| `clan.leader_cannot_leave` | Clan leader must use explicit dissolve in the current phase |
| `system.persistence_failed` | The authoritative runtime could not persist the required durable state |

## Sequence Examples

### Example A: Early `reject`

1. Client sends envelope with unsupported `protocol_version`
2. Server performs pre-validation
3. Server sends:

```json
{
  "kind": "reject",
  "command_id": "01JZBADVERS01",
  "command_seq": 3,
  "reason_code": "protocol.unsupported_version",
  "message": "Unsupported protocol version."
}
```

No `ack` is sent.

### Example B: `ack` Followed by `reject`

1. Client sends valid `use_skill`
2. Server passes pre-validation
3. Server sends `ack`
4. Server validates gameplay legality
5. Target is outside valid range
6. Server sends `reject`

```json
{
  "kind": "ack",
  "command_id": "01JZSKILL0002",
  "command_seq": 11,
  "status": "received"
}
```

```json
{
  "kind": "reject",
  "command_id": "01JZSKILL0002",
  "command_seq": 11,
  "reason_code": "combat.out_of_range",
  "message": "Target is out of range."
}
```

### Example C: `ack` Followed by `delta`

1. Client sends valid `move_intent`
2. Client starts reversible local movement prediction immediately after dispatch
3. Server passes pre-validation
4. Server sends `ack`
5. Server validates region and resolves movement through authoritative geodata/pathfinding
6. Server updates authoritative runtime position and active route
7. Server sends `delta` with the authoritative path and, if needed, `PositionCorrection`
8. Client blends prediction into the authoritative route or handles rejection/correction

```json
{
  "kind": "ack",
  "command_id": "01JZMOVE0002",
  "command_seq": 12,
  "status": "received"
}
```

```json
{
  "kind": "delta",
  "revision": 45,
  "applies_to_command_id": "01JZMOVE0002",
  "applies_to_command_seq": 12,
  "self": {
    "position": {
      "x": 12.4,
      "z": -1.2
    }
  }
}
```

## Anti-Examples

- Treating `ack` as gameplay success.
- Sending both `reject` and success `delta` for the same applied command.
- Using one undifferentiated error string instead of stable `reason_code` namespaces.
- Emitting `ack` before session attachment is verified.
- Replaying a stored command outcome to a socket whose ownership fence is stale.

## Invariants

- `ack` means pipeline acceptance, not gameplay success.
- `reject` means no authoritative state mutation from that command.
- `delta` is the success-path mutation output.
- Early protocol or session failures do not produce `ack`.
- Ownership loss rejects before `ack`, sequence advancement, or dedup reservation.
- Domain legality failures may produce `ack` followed by `reject`.
- Retry-safe replay of the same `session_id + command_seq + command_id` must not reapply side effects.

## Acceptance Criteria

- Every gameplay command follows the official lifecycle.
- Clients can distinguish `ack`, `reject`, and `delta` unambiguously.
- Reason codes are namespaced and stable.
- No handler conflates transport acceptance with gameplay application.
