# Server Terrain, Geodata, and Pathfinding

## Objective

Define the server-authoritative contract for terrain collision, geodata, navigability, and route generation.

This specification exists because click-to-move cannot be trusted if the client decides whether a coordinate is reachable. The client owns immediate presentation responsiveness, but the backend owns final movement truth.

## Decision

Movement remains target-point based:

- the player clicks a destination on terrain
- the client starts reversible local movement presentation immediately
- the client sends a `move_intent` with the destination point only
- the backend resolves the point against authoritative terrain/geodata without blocking client presentation
- the backend returns the accepted destination, route, or rejection/correction
- the client blends or corrects to the authoritative route and position

The client must not send authoritative paths, waypoints, collision results, or navmesh decisions.

## Responsive Hybrid Movement

Movement must feel immediate.

The player character should not stand still waiting for server pathfinding before locomotion starts. The client should begin local predicted movement as soon as the player clicks a valid-looking terrain point and dispatches `move_intent`.

The hybrid contract is:

1. Client raycasts the clicked terrain point and starts a reversible predicted move.
2. Client sends `move_intent` with only the destination point.
3. Backend acknowledges pipeline acceptance or rejects early according to the normal command lifecycle.
4. Backend resolves authoritative geodata/pathfinding with a bounded budget.
5. Backend returns authoritative route, snapped destination, or rejection through existing delta/correction contracts.
6. Client blends from predicted movement to the authoritative route instead of hard-stopping when possible.
7. Client stops, snaps, or returns to the last authoritative position only when the server rejects or the prediction drift is too large.

This is not a fallback authority model. It is client prediction plus server reconciliation.

### Prediction Leash

The client may move immediately, but it should keep prediction bounded.

Use a small prediction leash so latency does not let the local actor run far beyond server truth. If the authoritative path is delayed beyond the leash, the client may slow, ease, or hold near the predicted frontier while keeping input responsive.

The exact leash value is an implementation tuning parameter, but the product goal is:

- no visible one-second pause after click
- no long local run through blockers before correction
- no hard snap unless the server rejects or the client diverges badly

### Backend Concurrency

Goroutines or worker pools may be useful to keep server I/O and region loops responsive, but they are not the primary UX fix.

If pathfinding runs asynchronously in the backend:

- preserve `command_seq` ordering and dedup semantics
- cap CPU work per request
- cancel stale path work when a newer movement command supersedes it
- avoid unbounded goroutine creation
- do not mutate runtime position from multiple goroutines without a clear region actor, lock, or command-application boundary

The server may compute paths in the background, but the client still must not wait for the path before starting reversible presentation.

## Scope

The first mature implementation should support:

- static region terrain definitions
- static blocking obstacles such as rocks, walls, ruins, fences, props, and cliffs
- server-side navigability validation for a clicked destination
- server-side alternative route generation around blockers
- deterministic pathfinding over a region-scoped navigation representation
- authoritative movement deltas and position corrections
- tests proving that actors cannot move through blockers

Dynamic blockers such as moving players, doors, temporary walls, summons, or siege objects are future extensions. The interfaces should allow them later, but the first implementation should not overbuild around them.

## Authority Boundary

### Backend Owns

- region geodata version
- navigable surfaces
- blocked volumes
- static obstacle definitions
- actor collision profile
- movement profile
- pathfinding result
- destination snapping
- unreachable or blocked rejection
- final position and facing

### Client Owns

- raycast from screen to visible terrain
- provisional destination marker
- pending path preview as internal client state; visible path lines are debug-only
- immediate predicted locomotion for the local player
- animation interpolation between authoritative waypoints
- smooth blend from predicted motion to authoritative route
- blocked or unreachable feedback presentation

### Client Must Not Own

- collision legality
- final route
- route smoothing authority for gameplay truth
- obstacle bypass authority
- terrain layer authority
- geodata or navmesh override

## Region Geodata Model

Model geodata per region, not as one giant seamless world.

The first shipped slice now uses:

- a deterministic region-scoped grid
- versioned region geodata ids such as `clean_plain_1024_geo_v1`
- minimal static primitive blockers modeled as circles and rectangles, only when explicitly authored
- server-owned pathfinding that returns authoritative waypoint lists through existing `delta` payloads

The current active playable region still uses `stonecross_plaza` as a compatibility `region_id`, but its authored map content has been reset. It is now a clean prototype plain with no city, mobs, NPCs, buildings, props, roads, water, terrain overlays, or initial spawn content.

`clean_plain_1024_geo_v1` uses the default compact-region playable area of 1024x1024 world units, with bounds `x=-512..512` and `z=-512..512`. The client renderer, invisible ground raycast plane, server geodata bounds, spawn/checkpoint rules, and tests must all use this same region contract. Do not introduce hardcoded client clamps copied from old maps.

`clean_plain_1024_geo_v1` intentionally has no authoritative obstacles. The whole region is walkable until a new map concept is explicitly approved and implemented with matching renderer, picking plane, backend geodata, spawn/checkpoint, and tests. Legacy `dawn_plaza` runtime state may resolve to the same clean geodata while saves are migrated. New character creation may continue to checkpoint `stonecross_plaza` until the next canonical region id is defined.

Previously published local map assets remain available under `src/assets/maps`, but they are content library assets, not active map content unless wired through the documented map slice. Adding, moving, scaling, or rotating a GLB prop does not create an authoritative blocker unless the server geodata is explicitly changed and tested in the same slice.

### Region Definition Checklist

A new playable map is not complete when only the scene visuals exist. It is complete only when these pieces are created or updated together:

- canonical `region_id`
- canonical `geodata_version`
- playable bounds, defaulting to 1024x1024 world units for compact regions unless explicitly documented otherwise
- visible terrain surface sized to the same bounds
- invisible ground raycast/picking plane sized to the same bounds
- server geodata bounds using the same coordinate range
- spawn/checkpoint coordinates inside those bounds
- exits, portals, NPCs, mobs, loot, and grind zones placed inside those bounds when the approved map concept requires them
- frontend tests proving legal clicks near each intended edge remain legal
- backend tests proving movement toward every intended city exit or region edge is accepted or rejected for explicit authored reasons only
- renderer manifest entries for any local GLB assets used by the scene

Do not copy hardcoded clamps from an older map into a new scene. The reset specifically removed active map content and preserved only the shared 1024x1024 movement contract.

Target-driven actions such as skills, basic attack, and future interaction shortcuts must not path to the target center. They must resolve an authoritative destination that is walkable and still inside the action range, trying nearby candidate points around the target when a wall, building, prop, or target body blocks the direct approach point.

Each region geodata definition should be able to express:

- `region_id`
- `geodata_version`
- navigable cells, polygons, or triangles
- static obstacle shapes
- region bounds
- exit or portal edges to neighboring regions
- safe, combat, water, slope, blocked, and restricted area flags when needed
- actor radius or movement-profile constraints
- optional height information if verticality becomes relevant

The first implementation may use a deterministic grid, convex polygons, or a small navmesh. Choose the simplest representation that can:

- reject blocked destinations
- route around obstacles
- keep tests readable
- remain swappable behind a small interface

## Pathfinding Contract

Given:

- actor id
- actor current authoritative position
- actor movement profile
- current region id
- destination point
- current region geodata

The pathfinder must return one of:

- `accepted`: route and final destination
- `snapped`: route and nearest legal destination within allowed tolerance
- `rejected`: stable reason code when no legal path exists

The route should be a bounded waypoint list. It does not need to be a per-frame path.

The implementation must:

- validate the actor's starting point
- validate or snap the destination
- avoid blocked terrain and static obstacles
- obey actor radius or collision profile
- obey region bounds and allowed exits
- cap path length, node visits, and CPU budget
- produce deterministic results for the same inputs
- avoid route smoothing that cuts through blockers

## Movement Command Integration

`move_intent` remains point-based.

Valid payload:

```json
{
  "point": {
    "x": 12.4,
    "z": -1.2
  }
}
```

Invalid payload additions:

- `path`
- `waypoints`
- `collision_result`
- `navmesh_id`
- `geodata_override`

The backend derives route data after command pre-validation and before applying authoritative runtime movement.

The client may already be showing predicted movement by then. That prediction must remain reversible and must not change server truth.

## Outbound Contract

When a movement command succeeds, the server should return enough information for the client to reconcile:

- authoritative current position
- accepted destination
- route waypoints when movement is path-based
- facing when changed
- related command id and sequence
- optional correction reason when the destination was snapped or replanned

The exact transport shape should extend existing `delta` and `PositionCorrection` contracts rather than creating a parallel movement protocol.

## Reason Codes

Use stable reason codes:

- `movement.destination_blocked`
- `movement.destination_out_of_bounds`
- `movement.path_unreachable`
- `movement.path_budget_exceeded`
- `movement.geodata_unavailable`
- `movement.geodata_mismatch`

Use correction reasons:

- `path_resolved`
- `destination_snapped`
- `path_blocked`
- `path_unreachable`
- `movement_replanned`
- `geodata_mismatch`

## Runtime State

Runtime memory may hold:

- active path id or revision
- current route waypoints
- current waypoint index
- requested destination
- accepted destination
- geodata version used to resolve the path
- movement profile used for validation

Do not persist every waypoint advance.

PostgreSQL should keep durable position checkpoints at existing durability boundaries and may store static geodata content or version metadata when the content pipeline requires it.

## Persistence Guidance

Start with data structures that can later map cleanly to tables or content files.

Suggested persisted or content-backed concepts:

- `world_regions`
- `region_geodata_versions`
- `region_nav_surfaces`
- `region_static_obstacles`
- `region_portals`
- `movement_profiles`

Avoid storing per-frame movement or every path calculation in PostgreSQL.

Optional audit logs may record rejected movement attempts and suspicious repeated blocked movement when needed for abuse investigation.

## Client Presentation

The client should:

- start local predicted locomotion immediately after a terrain click and command dispatch
- show a pending marker immediately after terrain click when useful
- keep predicted and accepted route state distinct internally; visual route lines are debug-only and disabled in normal gameplay
- replace or blend the prediction with the authoritative server route
- show clear feedback when the destination is blocked or unreachable
- keep actor interpolation visually smooth without inventing new authority

The client may keep local collision and steering helpers for cursor affordances and smoother prediction, but those helpers are hints only.

## Combat and Interaction Dependencies

Combat range, loot pickup, NPC interaction, pet commands, and mounted movement should use authoritative runtime position produced by the pathfinding system.

Loot pickup must resolve to a walkable approach destination inside pickup range, not blindly to the visual center of the drop. If the direct destination produces a path endpoint outside pickup range, the server should select a deterministic nearby approach point around the loot and complete collection only from the authoritative tick loop.

Future line-of-sight and projectile rules may reuse geodata blockers, but they should be introduced as explicit combat validation rules instead of being hidden inside movement.

## Acceptance Criteria

- `move_intent` with a blocked destination is rejected or snapped with a stable reason.
- `move_intent` around a rock or wall produces an alternate route instead of moving through the obstacle.
- The local player starts moving immediately after click without waiting for the authoritative path response.
- The client blends from predicted movement to the server route without a visible one-second pause in normal latency.
- A wall that fully separates two areas makes the destination unreachable unless a valid opening exists.
- Actor radius is considered when routing near narrow gaps.
- Client-supplied paths or waypoints are rejected.
- `pick_up_loot` outside immediate range queues a server-owned approach route and collects only after the authoritative runtime enters pickup range.
- `pick_up_nearby` does not run a local retry loop; it selects a known loot id and delegates movement/range/persistence to the backend.
- Retry of the same movement command uses the existing command dedup contract and does not create divergent route outcomes.
- Runtime movement does not require PostgreSQL writes per frame.
- Unit tests cover grid/navmesh validation without network or database dependencies.
- E2E or integration tests prove browser click-to-move reconciles to the server route.
