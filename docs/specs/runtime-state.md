# Runtime State

## Objective

Define the authoritative runtime state model for the first online slice.

This document freezes:

- what exists in runtime memory
- what exists as durable truth in PostgreSQL
- when checkpoints are required
- what must not be persisted by microevent
- how `cooldown`, cast, target, position, and `known-set` are handled

## Decision

The first online slice uses a split authority model:

- runtime memory is authoritative for the present moment of online play
- PostgreSQL is authoritative for durable recovery state

The system does not attempt full replayable event sourcing and does not persist gameplay microstate by frame.

## Runtime Memory

The following data lives in authoritative runtime memory:

- active gameplay sessions
- connection-to-session binding
- session-to-character binding
- current runtime region membership
- current runtime position
- current facing
- current active movement path
- current path waypoint index
- current accepted movement destination
- current region geodata version used for movement validation
- current `known-set`
- current target state
- active cast state
- active cooldown map
- current owned pet roster
- current active companion known-set entity and mounted ownership link
- current party roster snapshot and pending invite projection
- current eligible party reward subset resolved at mob death
- current region-local entity presence
- region-local revision counters used for outbound deltas

### Motivation

- These values change too frequently to justify durable writes on every mutation.
- These values are needed on the hot path of validation and command application.
- These values are required to keep PostgreSQL out of frame-level gameplay loops.

## Durable Truth in PostgreSQL

The following data is durable and authoritative in PostgreSQL:

- accounts
- characters
- level
- XP
- durable HP
- durable MP
- durable position checkpoints
- durable region checkpoints
- versioned region geodata or static terrain content when persisted by the content pipeline
- durable skill cooldown end timestamps
- durable hotbar loadout snapshots
- durable pet ownership and summon or mount state
- durable party leader, membership, and pending invites
- minimum durable chat history and chat-command audit metadata
- inventory state
- equipment slot occupancy
- durable session records
- command deduplication records

The current implementation persists skill cooldown recovery state in `character_skill_cooldowns` keyed by `character_id + skill_id`.

The current implementation persists the first pet or mount slice in `character_pets` keyed by `pet_instance_id`, with `character_id`, `pet_template_id`, summon state, mount state, and timestamps.

The current implementation persists the first party slice in `parties`, `party_members`, and `party_invites`, keyed by `party_id`, `character_id`, and `invite_id`, with leader truth, membership, pending invite expiry, and timestamps.

The current implementation persists the first chat slice in `chat_messages`, keyed by `chat_message_id`, with sender, account, channel, optional target, optional region, sanitized text, and command metadata when available.

### Motivation

- These values must survive process restart or reconnect.
- These values participate in transactional game mutations.
- These values represent the recoverable truth of the character and inventory lifecycle.

## Boundary Between Memory and PostgreSQL

### Memory Is the Authority For

- the current online moment
- the current region presence graph
- the current `known-set`
- the live target reference
- the live cast in progress
- the live movement path, accepted destination, geodata version, and latest accepted coordinate

### PostgreSQL Is the Authority For

- recoverable character progression
- recoverable inventory and equipment state
- recoverable pet ownership plus summon or mount state
- recoverable party roster plus pending invites
- recoverable minimum chat history for audit and investigation flows
- recoverable cooldown end timestamps
- recoverable session records
- replay-safe command records keyed by `session_id + command_seq`
- last durable position and region checkpoint

### Explicit Non-Goal

PostgreSQL is not used as a frame-by-frame authority source for movement or presence.

## Checkpoint Policy

### Required Checkpoints

The system must checkpoint durable state on:

- character entry into the world
- region change
- logout
- session termination after disconnect grace handling
- death
- respawn
- any command that already opens a durable mutation boundary

The current simple respawn contract checkpoints the respawn position plus fully restored HP and MP whenever a persistence boundary is reached while the actor is dead.

### Durable Mutation Boundary Examples

- `use_skill` when it changes durable HP, MP, cooldown, XP, death, or loot outcomes
- `pick_up_loot`
- `use_item`
- `equip_item`
- `unequip_item`
- `interact_npc` when it changes quest truth or grants a reward
- `set_hotbar_state`
- `tame_mob`
- `summon_pet`
- `dismiss_pet`
- `mount_pet`
- `dismount_pet`
- `invite_party_member`
- `accept_party_invite`
- `decline_party_invite`
- `leave_party`
- `kick_party_member`
- `send_chat_message`

### Position Checkpointing

Position must be checkpointed:

- on `EnterWorld`
- on region change
- on logout
- on death or respawn
- on periodic coarse-grained flush

The periodic flush must be coarse-grained by time or displacement threshold.

The system must not checkpoint every accepted movement microstep.

## Microevents That Must Not Be Persisted

The following must not generate a durable write by themselves in the initial slice:

- each movement frame
- each intermediate coordinate
- each waypoint advance
- each path smoothing decision
- each pet follow presentation update
- each mounted companion visual position update
- each party HUD reorder or compact roster repaint
- each chat input focus, draft keystroke, or chat-tab switch
- each countdown decrement of a cooldown
- each decrement of remaining cast time
- each pointer movement during hotbar drag-and-drop
- each `known-set` appearance or disappearance
- each target selection change
- each visual reconciliation correction

## Treatment of Specific Runtime Concerns

### Cooldowns

Cooldowns are represented in runtime as:

- `skill_id -> cooldown_ends_at_ms`

Cooldowns are durable when they begin.

The system persists the end timestamp when the authoritative command starts the cooldown.

The current slice rehydrates those persisted cooldowns into `world/enter.self_state.cooldowns` and into attach-time runtime validation after restart or re-entry.

The system does not persist "cooldown ticked from 894 ms to 893 ms".

Cooldown expiration may be resolved lazily in runtime without an explicit durability write at the exact end moment.

### Cast State

Cast state is runtime-only in the initial slice.

The minimum cast state is:

- `skill_id`
- `target_id`
- `started_at_ms`
- `ends_at_ms`

On disconnect during cast, the initial slice cancels the cast rather than persisting in-flight cast recovery.

### Hotbar State

Hotbar layout is durable by full character snapshot, not by drag microevent.

The current online slice persists accepted `set_hotbar_state` commands in `character_hotbar_loadouts`.

The persisted snapshot includes:

- `open_bar_count`
- `slot_index`
- empty slot state
- `skill` binding by active learned `skill_id`
- `item` binding by owned `item_instance_id`
- `action` binding by supported action id such as `basic_attack` or `pick_up_nearby`

The current slice rehydrates this snapshot into `world/enter.self_state.hotbar`, attach-time runtime state, and runtime deltas after accepted rebinding.

The system does not persist every pointer movement during drag-and-drop.

Item bindings inside the hotbar remain durable shortcut truth only after `set_hotbar_state`, but gameplay activation still routes through the item's own authoritative command path:

- equipable item shortcut -> `equip_item`
- consumable item shortcut -> `use_item`

### Loot Pickup State

Loot pickup is authoritative runtime state until the collection boundary is reached.

When `pick_up_loot` targets a known loot entity outside immediate pickup range, the runtime may store a queued loot pickup with:

- command id
- command sequence
- loot entity id
- active server-resolved movement path toward a pickup-range destination

The client must not maintain a local retry queue for loot pickup. The authoritative tick loop advances movement, validates the final pickup range, persists the item mutation, removes the loot entity, and broadcasts the inventory delta plus `entity_disappear`.

If movement ends and the actor is still outside range, the runtime rejects with `world.loot_out_of_reach` rather than letting the client invent a second pickup attempt.

When the loot belongs to an eligible party reward subset, the runtime also keeps only the minimum in-memory ownership metadata:

- optional `party_id`
- eligible character ids resolved at kill time

This metadata gates pickup authority only. It is not a durable redistribution system.

### Quest State

Quest truth is durable gameplay state.

The current online slice persists the first authoritative quest vertical slice in `character_quests`.

The persisted quest snapshot includes:

- `quest_id`
- `status`
- `progress`

The current slice rehydrates this state into:

- `world/enter.self_state.quest`
- attach-time runtime quest validation
- runtime deltas after accepted quest progress, acceptance, or completion

NPC interaction prompts are runtime presentation state, not durable truth.

The runtime may project `npc_interaction` snapshots for merchant, warehouse, or quest dialog states, while the browser remains responsible only for rendering or dismissing the dialog presentation.

### Economy Audit State

Economy audit rows are durable investigation state, not runtime truth.

The current online slice persists vendor, exchange, warehouse, and player-trade mutations into `action_logs`, while warehouse item moves also persist into `storage_transfer_records`.

When the data is already cheap to compute inside the authoritative mutation, the persisted audit row may include:

- actor `character_id`
- actor `account_id`
- counterparty or target entity
- `item_instance_id`
- `template_id`
- quantity
- signed currency delta
- item or currency before and after snapshots
- stable `action_type`
- server timestamp
- `session_id`
- `command_id`
- `command_seq`

The client never authors these audit fields. They are derived only from the authoritative runtime plus the durable mutation boundary.

### Pet and Mount State

Pet ownership truth is durable state backed by PostgreSQL.

The current online slice persists:

- `pet_instance_id`
- `character_id`
- `pet_template_id`
- optional custom name
- `is_summoned`
- `is_mounted`
- timestamps

The current online slice rehydrates that state into:

- `world/enter.self_state.pets`
- attach-time runtime pet validation
- attach-time mounted move-speed derivation
- authoritative companion entities in `region_context` when a pet is currently summoned

The runtime does not persist per-frame pet position.

Simple follow presentation is derived from owner state plus authoritative summon or mount state. The backend remains authoritative for ownership, tame success, summon or dismiss, mount or dismount, and mounted speed.

### Party State

Party truth is durable gameplay and social state.

The current online slice persists:

- `party_id`
- `leader_character_id`
- party membership rows keyed by `party_id + character_id`
- pending invite rows keyed by `invite_id`
- invite expiry timestamps

The current online slice rehydrates that state into:

- `world/enter.self_state.party`
- `world/enter.self_state.party_invites`
- attach-time runtime party validation
- runtime deltas and `party_notice` messages for invite, accept, decline, leave, kick, and deterministic leader transfer
- kill-time party reward eligibility for shared XP and party-owned loot pickup

The runtime does not persist party HUD layout, member ordering by drag, or any frame-level roster repaint. Round-robin, master loot, dice distribution, clan, alliance, siege, and matchmaking remain outside this slice.

### Chat State

Chat delivery scope and rate-limit counters are runtime-only state.

The current online slice keeps in runtime:

- the active sender session binding
- the current region used for `region` chat fan-out
- current party membership snapshot used for `party` fan-out
- ephemeral burst rate-limit windows for chat spam protection
- dead state for combat legality, which does not block the current social chat slice

The current online slice persists minimum chat history in `chat_messages` with:

- sender `character_id`
- sender `account_id`
- `channel`
- sanitized `text`
- optional whisper `target_character_id`
- optional region id for `region`
- `session_id`
- `command_id`
- `command_seq`
- server `created_at`

The runtime does not persist offline-delivery queues, chat-tab UI state, or draft text. The browser remains responsible only for rendering escaped text and focusing the compact composer; it never decides delivery scope or whisper success.

### Target State

Target state is runtime-only in the initial slice.

Target state is not durable truth.

The selected target must be revalidated against:

- existence
- `known-set`
- current legality

### Position

Position is authoritative in runtime memory during live play.

PostgreSQL stores the last durable checkpointed position, not every intermediate position.

`PositionCorrection` reconciles client prediction to authoritative runtime position.

Movement paths are also authoritative in runtime memory during live play. A path may include the accepted destination, route waypoints, current waypoint index, movement profile, and geodata version used to resolve the route.

The runtime may replan or reject movement when server geodata says the destination is blocked or unreachable. It must not accept client-supplied waypoints as truth.

The client may show predicted locomotion before the authoritative route is ready. That prediction is client presentation state and is not part of authoritative runtime memory.

### Terrain and Geodata

Terrain/geodata is authoritative server content.

The runtime may keep immutable or versioned region geodata in memory for fast movement validation and pathfinding. PostgreSQL or content files may store the durable source or version metadata, but the database must not be queried for each movement frame.

Static geodata changes should be versioned. If a client view becomes stale, reconciliation should happen through authoritative deltas or `PositionCorrection`.

### Known-Set

`known-set` is runtime-only.

`known-set` is derived from:

- current region membership
- local relevance rules
- presence changes
- authoritative movement

Predicted local-only movement does not update `known-set`. `known-set` changes only from authoritative movement and presence rules.

`known-set` must not be stored as durable truth in PostgreSQL.

## Anti-Examples

- Persisting every movement update as a row write.
- Persisting every cast countdown mutation.
- Persisting `known-set` membership as durable state.
- Treating target selection as persistent truth that must survive restart.
- Clearing cooldown durability just because the cooldown expired in runtime.

## Invariants

- Runtime memory is authoritative for the present state of online play.
- PostgreSQL is authoritative for durable recovery state.
- Position is not persisted by frame.
- Movement paths are not persisted by waypoint.
- Client-supplied paths are never authoritative.
- Pet follow position is not persisted by frame.
- Party roster presentation is not persisted by frame.
- `known-set` is never a durable table-backed truth in the initial slice.
- Cooldowns are durable by end timestamp, not by countdown progression.
- Cast state remains volatile in the initial slice.
- Target state remains volatile in the initial slice.

## Acceptance Criteria

- The online runtime can validate commands without database reads on every spatial check.
- The online runtime can validate movement against server-owned region geodata.
- Durable writes happen on meaningful boundaries, not on microevents.
- Cooldown durability exists without per-tick persistence.
- `known-set`, target, and cast remain runtime-only.
- Position durability is checkpoint-based and not frame-based.
