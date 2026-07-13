# Implementation And Scope Guidance

## What This Reference Covers

The studied source represents a strong gameplay reference for this project.

`GameServer.java` loads a broad set of systems including:

- core MMORPG systems
- many event managers
- ranking systems
- timed item and drop diary systems
- fake player systems
- offline shop systems
- HWID and chat moderation systems
- marketplace and auction layers
- zone, boss, and dungeon systems

This makes it extremely valuable as a feature inventory, while still requiring architectural discipline in a new implementation.

## Major System Families Observed During Mapping

The bootstrap sequence and package layout show major families such as:

- PvP and arena events such as `TvT`, `CTF`, `DM`, `LM`, `FOS`, `KTB`, and tournament modules
- `TvTMoba` and other event structures
- daily rewards, missions, champion invasions, solo events, party farm, and solo boss events
- timed items, upgrade systems, fusion items, drop diary, and inventory effects
- community marketplace, auction modules, mail item flows, and ranking modules
- fake players, offline traders, and replay or recorder services
- HWID bonus campaigns, chat moderation, hero chat, VIP chat, and anti-abuse style modules

This is the main reason we should treat the source as a behavior and systems reference, not as a code transplant.

## Architectural Problems To Avoid Repeating

- Singletons own too many responsibilities.
- Packet handlers often contain business rules, event rules, and anti-abuse checks together.
- Entity classes call the database directly.
- Config flags branch core gameplay behavior in many places.
- Event rules inject themselves into base interaction flows.
- Live gameplay state and support tooling are not cleanly separated.

These patterns make change expensive and testing brittle.

## What To Preserve Conceptually

- character-centric domain model
- authoritative server movement and targeting
- layered actor model
- data-driven static content plus code-driven runtime resolution
- explicit party, clan, and territorial progression ladders
- long-lived competitive cycles for clan and hero prestige

## What To Rebuild Differently

Model the new game through bounded contexts:

- session and identity
- character profile and progression
- world presence and visibility
- movement and targeting
- combat and skill resolution
- PvE and loot
- items and economy
- quests
- party and clan
- PvP and competition
- territory and ownership
- live-ops events
- external communications such as transactional email

Each of these should have:

- explicit domain state
- clear command handlers
- small persistence boundaries
- metrics and audit hooks

## Recommended Service Boundaries

- `character-service` for progression and profile
- `world-service` for spawn, despawn, visibility, and zoning
- `combat-service` for attack, cast, effect, and death resolution
- `inventory-service` for item ownership, equipment, and storage
- `quest-service` for event-driven quest progress
- `social-service` for party, clan, alliance, and social permissions
- `competition-service` for PvP rules, ladders, and seasonal systems
- `territory-service` for castles, siege state, and territorial services
- `liveops-service` for opt-in events and special schedules

These can still live inside one modular monolith if the boundaries stay explicit.

## MVP Cut For Our Project

If we want a compact MMORPG inspired by Lineage but not crushed by implementation mass, the first slice should only include:

1. login and character selection
2. world spawn and region presence
3. click-to-move and target click
4. basic monsters and NPC interaction
5. single-target combat
6. skill casting with early AoE support
7. loot and inventory
8. equipment and stat updates
9. one city and one or two short field regions
10. simple quests and party support

The next slice can add:

1. class progression and stronger skill trees
2. warehouses, shops, and teleporters
3. PvP flagging and karma rules
4. clan creation and simple clan progression
5. an initial tameable-companion feature for selected monster families

Only after that should we consider:

1. clan territory systems
2. olympiad-like competitive ladders
3. hero prestige
4. Seven Signs or manor-like metas
5. broader event catalogs

## Explicit Defer List

These are useful reference systems, but they should be deferred in a clean rebuild:

- Seven Signs
- festival chains
- manor economy
- boats and travel networks
- petition systems
- hero diary UI and all-time hall of fame
- offline shop restoration
- fake players
- the broader event catalog from the studied source

## Data Guidance

Do not reproduce the old pattern where `Player.java` owns every persistence concern.

Prefer:

- one current-state table set per aggregate
- focused audit trails for economy and competition
- replayable combat or command logs only where they create real value
- static content from files or data packs, not from mutable transactional tables

## Security And Integrity Guidance

Do not copy anti-abuse checks into arbitrary packet handlers.

Prefer:

- authoritative command validation
- transport-independent anti-cheat and abuse policies
- structured audit logs for economy, PvP, and territory
- explicit provider boundaries for email or external systems

## QA Guidance

The old codebase shows exactly where regressions accumulate:

- target legality
- zone-based exceptions
- item restriction rules
- subclass transitions
- reward distribution
- clan and siege state transitions

These should become simulation and property-test priorities in the new codebase.

## Source Anchors

- `java/net/sf/l2j/gameserver/GameServer.java`
- `java/net/sf/l2j/gameserver/model/actor/Player.java`
- `java/net/sf/l2j/gameserver/network/clientpackets/UseItem.java`
- `java/net/sf/l2j/gameserver/network/clientpackets/Action.java`
- `java/net/sf/l2j/gameserver/model/entity/Siege.java`
- `java/net/sf/l2j/gameserver/model/olympiad/Olympiad.java`
