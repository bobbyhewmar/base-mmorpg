# Region Presence and Known-Set

## Objective

Define the minimum authoritative model for region presence and `known-set` in the first online slice.

This document freezes:

- the minimum region model
- the minimum presence model
- the meaning of `known-set`
- entry and exit rules for `known-set`
- gameplay validations that depend on `known-set`
- the minimum contracts for `RegionContext`, `EntityAppear`, `EntityDisappear`, and `PositionCorrection`

## Decision

The first online slice uses region-scoped authoritative runtime presence.

Each active gameplay session belongs to exactly one primary region context at a time.

Each session owns a runtime `known-set` consisting of relevant entities currently visible and interactable enough to be legally referenced by commands.

`known-set` is a runtime concept. It is not persisted as durable truth.

## Minimum Region Model

A region is the minimum authoritative spatial bucket for:

- player presence
- NPC presence
- mob presence
- loot presence
- terrain/geodata version context
- movement pathfinding and collision validation
- relevance evaluation
- presence-scoped outbound messages

The initial slice uses static region definitions already aligned to the compact world structure.

The region model is not a seamless-world streaming system and does not introduce cross-shard routing.

The cross-instance foundation adds a durable ownership lookup without changing this spatial model. It can distinguish a ready local player from a player with an unexpired lease on another instance, but it does not fan out remote movement or synthesize remote visibility.

## Minimum Presence Model

Presence is the authoritative runtime membership of an entity inside a region context.

The initial presence model covers:

- player characters
- NPCs
- mobs
- loot entities

For player-character entities, the first shipped presence payload uses:

- `entity_type: player`
- a project-owned `template_id` for the generic remote-player visual
- runtime `state` fields for identity, `level`, `cp`, `hp`, `dead`, `facing`, temporary PvP flag projection, durable PvP/PK counters, and karma

Presence exists to answer:

- which region an entity belongs to
- which sessions should know about the entity
- which commands may legally reference the entity

## Definition of `known-set`

`known-set` is the runtime set of entities currently known to a session for authoritative interaction purposes.

An entity in `known-set` is:

- present in a relevant region context
- currently relevant to the session
- valid to reference for further legality checks

`known-set` does not guarantee that the action is legal.

`known-set` is necessary but not sufficient for:

- targeting
- skill use
- NPC interaction
- loot pickup

Additional legality checks still apply, such as range, state, and command-specific rules.

Client-side predicted movement does not expand `known-set`.

For a known player, the server also resolves ownership scope at command time:

- `local`: ownership and fence match a ready runtime on this instance
- `remote`: an unexpired lease belongs to another instance
- `offline`: no unexpired lease exists
- `unavailable`: the durable owner is this instance but its matching ready runtime is absent

Ownership scope is not `known-set` membership. It cannot make an unknown player targetable.

The local player may visually start moving before authoritative path resolution, but entity relevance and command legality must continue to use authoritative runtime position and presence state.

## Entry Rules for `known-set`

An entity enters a session `known-set` when one or more of the following becomes true:

- the session enters a region and the entity is already relevant there
- the entity enters the session's relevant region scope
- the entity moves into local relevance range
- the entity spawns within local relevance range
- the entity becomes newly relevant after an authoritative state change

Predicted local-only movement is not an entry rule.

When an entity enters `known-set`, the server must emit `EntityAppear`.

## Exit Rules for `known-set`

An entity leaves a session `known-set` when one or more of the following becomes true:

- the entity leaves local relevance range
- the session changes region
- the entity changes region
- the entity is removed from the world
- the entity becomes non-relevant to the session

When an entity leaves `known-set`, the server must emit `EntityDisappear`.

## Validations That Depend on `known-set`

The following commands require the referenced entity to be in the session `known-set` at validation time:

- `select_target`
- `use_skill` when the payload references `target_id`
- `basic_attack`
- `interact_npc`
- `pick_up_loot`

If the entity is not in `known-set`, the command must be rejected with a stable reason code such as:

- `world.entity_not_known`

## Targeting and Interaction Rules

### Targeting

`select_target` is valid only when:

- `target_id` exists
- the target entity is in the current `known-set`
- the entity is targetable by the command type

### Skill Use

`use_skill` with `target_id` is valid only when:

- the target remains in `known-set`
- range is valid
- cooldown is not active
- cost can be paid
- command-specific combat legality passes

For a player target, known-set membership does not authorize damage. The PvP path also requires live attachment in the same region, an open region policy, distinct living characters, non-matching party and clan membership, authoritative range, and the command-specific skill/resource rules in `docs/specs/pvp-pk.md`.

If the known player is online on another instance, `select_target` and PvP reject with `presence.target_remote`. There is no local target success, remote combat mutation, or delivery fallback. A party/clan invite may continue only when its recipient came from an already authoritative player target that drifted remote; the command revalidates durable ownership and social eligibility and routes only the resulting lifecycle notice through the outbox. This never adds the remote player to `known-set`.

### Basic Attack

`basic_attack` requires a live known target. Mob targets keep the existing PvE approach/auto-attack behavior. Player targets use the separate immediate PvP eligibility path and do not auto-approach or auto-repeat in the first slice.

### NPC Interaction

`interact_npc` is valid only when:

- the NPC is in `known-set`
- the NPC is interactable
- interaction range is valid

### Loot Pickup

`pick_up_loot` is valid only when:

- the loot entity is in `known-set`
- the loot is still present
- the actor is inside pickup range, or the server can resolve a canonical loot-approach path to a walkable point inside pickup range
- loot reservation or legality rules pass

The client must not retry `pick_up_loot` based on locally projected distance. Clicking a drop or activating `pick_up_nearby` sends one authoritative command. If the loot is known but not immediately reachable, the runtime queues movement toward a server-selected approach point and completes the pickup on an authoritative tick once the actor enters range.

## Contract: `RegionContext`

`RegionContext` is the minimum initial authoritative region snapshot sent when:

- the session enters the world
- the session changes region
- the server needs to rebuild region-local state for the client

`known_entities` may now contain `player` entities for other attached characters already relevant in the same region.

Example:

```json
{
  "kind": "region_context",
  "region_id": "stonecross_plaza",
  "region_revision": 21,
  "geodata_version": "clean_plain_1024_geo_v1",
  "self_position": {
    "x": -8,
    "z": 0
  },
  "known_entities": [
    {
      "entity_id": "npc_wardkeeper",
      "entity_type": "npc",
      "template_id": "wardkeeper",
      "position": {
        "x": -2,
        "z": 10
      },
      "state": {}
    }
  ]
}
```

### Required Fields

| Field | Meaning |
| --- | --- |
| `kind` | Must be `region_context` |
| `region_id` | Current authoritative region |
| `region_revision` | Monotonic region revision for the session view |
| `geodata_version` | Authoritative terrain/geodata version used for movement reconciliation |
| `self_position` | Authoritative self coordinate in the region |
| `known_entities` | Initial entity set relevant to the session |

## Contract: `EntityAppear`

`EntityAppear` announces that an entity entered the session `known-set`.

This includes other player characters entering the same authoritative region scope.

Example:

```json
{
  "kind": "entity_appear",
  "region_revision": 22,
  "entity": {
    "entity_id": "mob_1",
    "entity_type": "mob",
    "template_id": "mireling",
    "position": {
      "x": 34,
      "z": 10
    },
    "state": {
      "hp": 54,
      "alive": true
    }
  }
}
```

### Required Fields

| Field | Meaning |
| --- | --- |
| `kind` | Must be `entity_appear` |
| `region_revision` | Monotonic region revision for ordering |
| `entity` | Entity payload entering `known-set` |

## Contract: `EntityDisappear`

`EntityDisappear` announces that an entity left the session `known-set`.

Example:

```json
{
  "kind": "entity_disappear",
  "region_revision": 23,
  "entity_id": "mob_1",
  "reason": "out_of_relevance"
}
```

### Required Fields

| Field | Meaning |
| --- | --- |
| `kind` | Must be `entity_disappear` |
| `region_revision` | Monotonic region revision for ordering |
| `entity_id` | Entity leaving `known-set` |
| `reason` | Stable disappearance reason |

Initial disappearance reasons:

- `out_of_relevance`
- `region_changed`
- `despawned`
- `removed`

## Contract: `PositionCorrection`

`PositionCorrection` reconciles client prediction against authoritative runtime position.

Example:

```json
{
  "kind": "position_correction",
  "applies_to_command_seq": 18,
  "position": {
    "x": 12.4,
    "z": -1.2
  },
  "facing": 1.57,
  "reason": "authoritative_reconcile"
}
```

### Required Fields

| Field | Meaning |
| --- | --- |
| `kind` | Must be `position_correction` |
| `applies_to_command_seq` | Command sequence most directly related to the correction |
| `position` | Authoritative corrected position |
| `facing` | Authoritative facing |
| `reason` | Stable correction reason |

Initial correction reasons:

- `authoritative_reconcile`
- `region_transition`
- `movement_clamped`
- `path_resolved`
- `destination_snapped`
- `path_blocked`
- `path_unreachable`
- `movement_replanned`
- `geodata_mismatch`

## Anti-Examples

- Treating region membership as a UI-only concern.
- Allowing `target_id` references to bypass `known-set`.
- Persisting `known-set` membership in PostgreSQL as durable truth.
- Using `EntityAppear` only for rendering while ignoring its legality role.

## Invariants

- Each active session belongs to one primary region context at a time.
- `known-set` is runtime-only.
- durable session ownership classifies local, remote-online, and offline without persisting visibility.
- Entity-referencing commands depend on `known-set`.
- `known-set` membership is not enough by itself to make an action legal.
- `RegionContext`, `EntityAppear`, `EntityDisappear`, and `PositionCorrection` are authoritative transport contracts.
- `RegionContext.geodata_version` is the authoritative client context for movement reconciliation, not permission for the client to decide paths.

## Acceptance Criteria

- The server can construct and maintain `known-set` per session.
- Entity-referencing commands are rejected if the entity is not in `known-set`.
- Region entry and exit produce stable presence events.
- Client reconciliation uses authoritative `PositionCorrection`.
- No durable table is required to store `known-set` state.
