# Delivery Roadmap

## Phase 0: Foundation and Direction

Goal:
Align on the compact MMORPG direction, world structure, system boundaries, and production principles before major implementation begins.

Exit criteria:

- architecture direction agreed
- world structure agreed
- first skill set updated
- unresolved gameplay questions captured
- MVP slice defined

## Phase 1: Local World Prototype

Goal:
Prove the web-first compact MMORPG client with one city hub, one nearby field region, 3D characters, hybrid HUD, and the core interaction language without online complexity.

Exit criteria:

- playable city and field navigation
- constrained elevated camera tuned for readability
- hybrid HUD showing player state, target state, hotbar, and inventory outside the 3D scene
- classic compact HUD language established for login/register, player status, target frame, chat, hotbar, inventory, and the character-window family
- inventory opened by `ALT+V`
- character windows opened by `ALT+T` Status, `ALT+K` Skills, `ALT+C` Actions, `ALT+N` Clan, and `ALT+U` Quests
- compact `32x32px` item and skill icon grids with hover/focus tooltips
- browser runtime works as the canonical client path
- movement, targeting, loot, and NPC interaction feel coherent
- terrain blockers and walkable routes are visually readable even before online authority is added
- early UX feedback captured

## Phase 2: Online MVP

Goal:
Ship an authoritative Massive Multiplayer Online slice with secure account access, explicit character entry, compact region traversal, combat, loot, and persistence.

Exit criteria:

- registration, login, and account verification flow
- character list and character creation flow
- race-first character creation with authoritative `Fighter` or `Mage` base-class validation
- secure session attach between client and backend
- authoritative movement and action handling
- server-authoritative terrain/geodata pathfinding for movement around static obstacles
- persistent character state
- authoritative skill-book and hotbar projection
- HUD-local drag of active skills and inventory items to shortcut/action bar slots with the icon following the cursor, with durable online rebinding still requiring a backend command
- future action shortcuts from `ALT+C`, including `basic_attack`, require authoritative commands before gameplay use
- one city-to-field progression loop
- observability dashboards
- rule coverage for core combat and economy loops

## Phase 3: Production Hardening

Goal:
Raise confidence for live operation, abuse resistance, and moderate concurrency.

Exit criteria:

- load-test targets met
- operational runbooks in place
- recovery drills completed
- security review findings addressed

## Phase 4: Expansion

Goal:
Add breadth only after the compact core loop proves stable.

Candidate areas:

- additional cities and regions
- party and guild features
- improved search and content discovery
- Redis-backed presence and fan-out
- advanced analytics pipelines
