# ADR-001: Online Slice Authority Boundaries

## Status

Accepted

## Context

The project is transitioning from a browser-playable local prototype to an authoritative online slice.

The current prototype proves the core loop, but it still concentrates gameplay authority in the client runtime. The online transition must preserve the validated loop while removing authority duality between client and server.

The project already accepts the following macro decisions:

- Backend remains a Go modular monolith.
- PostgreSQL remains the durable store of record.
- The browser remains the canonical client runtime.
- The client remains hybrid: Three.js for world rendering and HTML/CSS for HUD.
- Redis, microservices, event sourcing, and broad social scope remain out of scope for the initial online slice.

This ADR freezes the authority boundaries for the first authoritative online slice and the split between `Fase 1.1` and `Fase 1.2`.

## Decision

The first authoritative online slice adopts the following decisions:

1. Gameplay authority moves to the backend per migrated command.
2. The actor of a gameplay command is derived from the authenticated connection and active gameplay session, never from client-provided gameplay identity fields.
3. Gameplay commands use one official envelope with `command_id` and `command_seq`, while the server answers through authoritative `ack`, `reject`, and `delta` messages.
4. Runtime online state is split between:
   - volatile authoritative runtime state in memory
   - durable authoritative state in PostgreSQL
5. Region presence and `known-set` are first-class runtime concepts and are required for targeting and interaction legality.
6. The pre-game flow becomes mandatory: register or log in, then select or create a character, then enter the world.
7. Character creation is server-authoritative for race, base class, sex, hairstyle, skin type through the persisted `skin_type` field, normalized name, and entry permission.
8. Client-supplied prices, total costs, or economy outcomes are never accepted as truth.
9. `Fase 1.1` proves account flow, session, region, presence, movement, targeting, and protocol semantics.
10. `Fase 1.2` extends the same authority model to combat resolution, loot, inventory, and equipment.
11. Mature movement resolves terrain collision and pathfinding through server-owned region geodata, not client-provided paths.

## Motivation

- Remove duality of authority between client and server.
- Prevent spoofing of gameplay actor identity.
- Prevent PostgreSQL from becoming a frame-by-frame runtime engine.
- Freeze a stable protocol before backend implementation.
- Enable incremental migration without rearchitecting the client.
- Keep the online slice small enough to prove architecture before expanding scope.

## Authority Boundaries

### Client Authority

The client is authoritative only for:

- local input capture
- local camera state
- local UI state that does not decide gameplay legality
- temporary visual prediction and reversible UX feedback
- rendering of authoritative world and HUD projections

The client is not authoritative for:

- session identity
- gameplay actor identity
- account verification outcome
- character-creation legality
- character-name validity or uniqueness
- accepted position
- movement path, waypoints, collision legality, or terrain navigability
- region membership
- `known-set`
- target legality
- skill legality
- damage or effect resolution
- item prices, total costs, or economy result values
- loot ownership or pickup success
- inventory truth
- equipment truth

### Backend Authority

The backend is authoritative for:

- authenticated account context
- active gameplay session binding
- actor derivation for gameplay commands
- account registration and login legality
- character creation, race-base-class-sex validation, and name reservation
- region presence
- `known-set`
- accepted movement
- terrain/geodata pathfinding
- movement destination snapping and blocked-path rejection
- target legality
- command validation
- command application
- damage, effects, cooldowns, and cast legality
- loot spawn and pickup legality
- inventory membership
- equipment slot occupancy
- item valuation, taxes, discounts, and resulting currency mutations
- durable persistence boundaries

### PostgreSQL Authority

PostgreSQL is authoritative for durable state only.

PostgreSQL is the durable source of truth for:

- accounts
- characters
- persisted character-creation facts and name uniqueness
- persisted HP and MP
- XP and level
- persisted position and region checkpoints
- inventory state
- equipment slot occupancy
- sessions as durable records
- command deduplication records
- versioned geodata or static terrain content when persisted by the content pipeline

PostgreSQL is not the operational authority for:

- per-frame movement
- `known-set`
- per-tick cast progress
- per-tick cooldown countdown
- transient target selection

## Invariants

- Each migrated gameplay command has exactly one authority.
- The actor of a gameplay command is derived from the active authenticated session.
- `character_id` is not accepted inside gameplay command envelopes.
- `ack` never means gameplay success.
- Gameplay success is represented by authoritative `delta`.
- The client never creates an account, a character, or an economy result by authority; it only requests them.
- `known-set` is required for entity-referencing commands.
- Runtime memory is authoritative for the present moment of online play.
- PostgreSQL is authoritative for durable recovery state.
- Position is not persisted by frame or by micro-movement event.
- Client-supplied paths, waypoints, and collision results are never authoritative.
- The system does not keep a generic fallback that lets local client rule execution remain authoritative once a command is migrated online.

## Fase 1.1 Scope

`Fase 1.1` includes:

- account registration and login
- character list retrieval
- character creation
- character entry
- active gameplay session binding
- initial `RegionContext`
- region presence
- `known-set`
- `move_intent`
- authoritative position acceptance
- server-side geodata/pathfinding when movement is matured beyond the first coordinate-acceptance slice
- `PositionCorrection`
- `select_target`
- command lifecycle with `ack`, `reject`, and `delta`
- durable session and position-region checkpoints

`Fase 1.1` acceptance objective:

- prove that the server is the only gameplay authority for session, movement, region presence, and targeting
- prove that direct cold start into the world is gone and replaced by authenticated login plus explicit character entry
- prove that the command protocol is stable enough to carry later combat commands without redesign

## Fase 1.2 Scope

`Fase 1.2` includes:

- `use_skill`
- cooldown application
- cast state
- damage and effects resolution
- mob death
- loot spawn
- `pick_up_loot`
- server-owned loot approach when the drop is outside immediate pickup range
- inventory mutation
- equipment mutation
- persistence of combat outcomes and related durable state

`Fase 1.2` acceptance objective:

- prove that the same authority model extends to the full core loop without reintroducing client authority

## Anti-Scope

The following remain outside this ADR and outside the first online slice:

- microservices
- Redis in the critical gameplay path
- event sourcing
- social systems beyond minimal session presence
- party systems
- clan or guild systems
- broad PvP systems
- broader trade and warehouse variants beyond the current minimum authoritative slice
- reconnect sophistication beyond minimal session lifecycle handling
- multiple cities or seamless-world expansion
- database writes per frame of movement
- trusting client-supplied paths, waypoints, or collision results
- trusting client-supplied item prices or currency totals

## Consequences

### Positive

- The online slice gains stable command identity and validation semantics.
- The project avoids spoofable gameplay actors.
- Runtime and persistence concerns remain separable.
- `Fase 1.1` can be implemented without combat scope creep.
- `Fase 1.2` can reuse the same protocol and runtime boundaries.

### Negative

- The client must stop mutating gameplay state directly for migrated commands.
- Runtime memory becomes the operational authority for online presence and movement.
- Some state remains intentionally volatile and must be recovered through checkpoints rather than exact replay.

## Risks Accepted

- Cooldown durability is retained while cast state remains volatile in the initial slice.
- Disconnect during active cast cancels the cast instead of attempting full in-flight recovery.
- Target state remains runtime-only and may be cleared on reconnect or state rebuild.
- Position may roll back to the latest durable checkpoint rather than the exact last in-memory coordinate after failure.
- `Fase 1.1` intentionally defers combat to avoid mixing protocol stabilization with combat-state complexity.

## Related Documents

- `docs/specs/command-envelope.md`
- `docs/specs/account-auth-and-character-entry.md`
- `docs/specs/command-lifecycle.md`
- `docs/specs/runtime-state.md`
- `docs/specs/region-presence-known-set.md`
- `docs/specs/server-terrain-geodata-pathfinding.md`
- `docs/backlog/online-slice-now-next-later.md`
