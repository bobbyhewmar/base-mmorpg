# Architecture Overview

## Current Direction

Adopt a Linux-hosted, Go-based modular monolith with PostgreSQL as the authoritative data store and a web-first hybrid MMORPG client: Three.js for the world view and HTML/CSS for the main HUD.

The product direction is a compact MMORPG built around city hubs and short surrounding territories, not a giant open world and not a turn-based match structure.

## Major Components

### Hybrid game client

- Render a compact 3D world with 3D characters and world interaction layers.
- Show authoritative character and nearby-world state received from the backend.
- Keep the main HUD in HTML/CSS for clarity, responsiveness, and maintainability.
- Keep client-only state limited to camera, transient targeting presentation, animation, local drafts, and reversible UI state.
- Treat the browser as the canonical runtime and keep desktop shells optional.
- Keep runtime-specific features behind a platform bridge.
- Support click-to-move terrain navigation and click-to-target combat as the core interaction model.
- Treat movement routes, collision, and terrain navigability as backend-authoritative; the client may predict movement immediately but must not decide final route truth.
- Treat hostile skill use as target-locked by default: select mob first, then execute the skill.
- Preserve lowpoly world readability while still supporting visually meaningful equipment changes on the character model.
- Do not let the client become authoritative for identity, gameplay legality, economy values, inventory truth, or progression outcomes.
- Treat every client payload as untrusted input that must be validated, normalized, and either accepted or rejected by the backend.

### Account and entry flow

- Require an explicit pre-game flow: register or log in, then select or create a character, then enter the world.
- Keep account registration, login, session issuance, and character-entry permission fully authoritative on the backend.
- Drive character creation from server-owned content and validation rules.
- Let the player choose race, base class, sex, hairstyle, hair color, face, and name in the client UI, but validate every option combination on the backend before the character exists.
- Start the first online slice with race-first creation and initial base class choice limited to `Fighter` or `Mage`, with the backend deciding which combinations are legal per race.
- Treat selected appearance as persisted character state that must flow unchanged into lobby visuals, world bootstrap, and remote player presence.
- Treat character name availability, normalization, profanity rules, and uniqueness as backend-only concerns.
- Do not allow direct world entry from a cold client without an authenticated account flow and a valid character-entry path.

### Three.js scene layer

- Render cities, short field regions, enemies, NPCs, loot, and exits.
- Use a constrained elevated camera for high readability unless prototype evidence points elsewhere.
- Keep diegetic overlays in-world, such as selection rings, interaction markers, drop markers, and combat feedback.
- Render pending and authoritative movement routes distinctly when the backend returns path data.
- Start local-player movement presentation immediately on terrain click and reconcile smoothly to the server route.
- Support modular character visuals so authoritative gear state can change weapons, armor silhouette, and major worn pieces in runtime.

### HTML/CSS HUD layer

- Render player state, target state, hotbar, inventory, minimap, quests, and chat.
- Handle complex layout, text-heavy UI, tooltips, modals, and responsive behavior.
- Remain visually synchronized with authoritative state without becoming a second gameplay authority.
- Support companion and mount status panels when the character has an active pet or mount.
- Support a persistent multi-row compact `32x32px` skill hotbar and a skill-book popup without moving skill authority to the client.
- Keep inventory, skill book, status, chat, and future modal windows on the shared classic HUD visual language: square corners, dark body, blue title bars, icon grids, and real clickable controls.

### World structure model

- Organize the world into compact city hubs and short adjacent regions.
- Keep progression and spatial risk legible by region.
- Favor region-scoped logic and bounded local entity density over giant seamless-world assumptions.
- Keep terrain, blockers, exits, and navigable surfaces region-scoped so server pathfinding stays simple and measurable.

### Server terrain, geodata, and pathfinding

- Treat terrain collision and pathfinding as server-side gameplay rules, not client-side rendering helpers.
- Keep `move_intent` destination-based; the client sends the clicked point, not a path.
- Resolve movement through region geodata that knows navigable surfaces, static obstacles, bounds, and exits.
- Generate alternate routes around blockers such as rocks, walls, ruins, fences, and cliffs.
- Keep pathfinding non-blocking from the player's point of view by relying on client prediction plus server reconciliation.
- Return authoritative path, destination, rejection, or correction through the existing command lifecycle.
- Keep geodata implementation swappable behind a small domain interface so the first version can be simple without locking the architecture.

### Companion and mount system

- Support taming selected monsters and converting them into persistent companion instances.
- Allow species-specific roles such as combat pet, utility companion, or mount.
- Keep companions and mounts authoritative on the backend as first-class actor variants.
- Let mounts alter movement presentation and movement rules without turning movement into a separate control model.
- Keep taming eligibility, active companion limits, and mount restrictions explicit in game rules.

### Backend application

- Expose HTTP and WebSocket endpoints.
- Authenticate users and authorize access to characters and sessions.
- Issue and validate secure account and gameplay session tokens.
- Accept only HTTPS and WSS traffic in online environments.
- Validate player actions such as movement, combat, loot, and NPC interactions.
- Validate movement through server-owned terrain/geodata and pathfinding before mutating runtime position.
- Validate inventory actions such as equip, unequip, use item, drop, destroy, storage transfer, and exchange before mutation.
- Validate companion actions such as tame attempts, summon or unsummon, mount or dismount, and pet combat behavior.
- Resolve authoritative prices, taxes, vendor offer values, and item legality on the backend instead of trusting client-supplied numbers.
- Execute game rules in deterministic gameplay modules.
- Persist state transitions and emit domain events.

### Item and inventory system

- Keep immutable item templates separate from mutable item instances.
- Keep container membership and equip-slot occupancy authoritative instead of duplicating item placement truth.
- Support shared mutation semantics across inventory, storage, trade, and exchange flows.
- Keep equip and unequip side effects explicit through hooks or events for stats, skills, penalties, and visible gear state.
- Treat item price, vendor price, taxes, discounts, and total currency deltas as backend-derived values only.

### Transactional email integration

- Treat transactional email as an asynchronous platform capability, not part of the gameplay critical path.
- Start with `Resend` for provider delivery.
- Drive emails from committed domain events and notification intents.
- Ingest provider webhooks back into the platform for delivery observability and support.

### PostgreSQL

- Store accounts, characters, inventories, progression, quests, notifications, and audit trails.
- Store job tables for low-volume asynchronous work in early phases.
- Support debugging and abuse investigation through action logs plus selective snapshots later.
- Store durable account-security and character-entry records needed for registration, verification, login, name uniqueness, and session traceability.
- Store or reference versioned region geodata, static obstacles, and movement profiles when the content pipeline needs durable terrain data.

### Optional Redis

Add Redis only for measured ephemeral concerns:

- presence
- rate limiting
- WebSocket fan-out across multiple backend instances
- short-lived read caching outside the write path

Do not use Redis as the source of truth for progression, economy, or live gameplay authority.

### Optional search tier

Start with PostgreSQL full-text search for item, lore, support, and admin lookup use cases. Consider Elasticsearch or a similar engine only if search relevance, fuzzy matching, or indexing volume becomes a real product requirement.

## World And Interface References

Read:

- [world-structure.md](world-structure.md)
- [interface-architecture.md](interface-architecture.md)
- [combat-and-targeting.md](combat-and-targeting.md)
- [client-runtime-strategy.md](client-runtime-strategy.md)
- [dependency-boundaries.md](dependency-boundaries.md)
- [specs/server-terrain-geodata-pathfinding.md](specs/server-terrain-geodata-pathfinding.md)
- [specs/account-auth-and-character-entry.md](specs/account-auth-and-character-entry.md)
- [specs/hud-skills-and-hotbars.md](specs/hud-skills-and-hotbars.md)
- [specs/hud-inventory-and-classic-windows.md](specs/hud-inventory-and-classic-windows.md)

## Runtime Flow

### Authoritative player-action path

1. Accept a player action over HTTP or WebSocket.
2. Authenticate and authorize the actor.
3. Translate transport input into an internal command.
4. Load the relevant character, session, and nearby-world context.
5. Execute rule validation and state transition logic, including terrain/geodata pathfinding for movement.
6. Persist the new state and emitted events in one transaction.
7. Publish the resulting delta through outbound adapters.
8. Emit metrics, traces, and structured logs for the full path.

### Asynchronous work

Keep asynchronous work out of the critical path when it does not affect immediate gameplay:

- notifications
- analytics export
- search indexing
- replay or audit packaging
- email and account workflows

Use PostgreSQL-backed jobs first. Promote to a dedicated queue only after throughput or operational isolation demands it.

### Transactional email flow

Read [transactional-email-integration.md](transactional-email-integration.md) for the detailed email workflow.

## Scaling Assumptions

This product remains a Massive Multiplayer Online game, but the first online architecture should reach that category through authoritative region-scoped concurrency, not through speculative giant-world distribution from day one. Two thousand active players is still small enough to avoid speculative distributed architecture. Scale by:

- keeping transactions short
- scoping interest and updates by region
- caching immutable or versioned region geodata in backend runtime instead of reading it per movement frame
- serializing conflicting actions at character or contested-entity boundaries
- sizing connection pools conservatively
- using PgBouncer
- adding read replicas only when read pressure is real
- load-testing the actual action mix instead of guessing from registered-user counts

## Non-Goals For Early Phases

- giant open-world simulation
- microservices
- multi-database truth for gameplay
- cross-region active-active gameplay
- event streaming platforms in the core path
- complex cache invalidation schemes
