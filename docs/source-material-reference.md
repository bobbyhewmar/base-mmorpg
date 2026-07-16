# Source Material Reference

Use this document as the baseline for the project's visual and spatial direction after the pivot to a compact MMORPG.

## Default Source Study Reference

Use `D:\Jogos\Lineage II\Servidores\Lucera\Souce\main` as the default direct study source before implementing any feature that has a clear Lineage 2 equivalent.

The source is a concept and behavior reference, not a codebase to port. Extract responsibilities, lifecycle, validations, state transitions, edge cases, data ownership, and feature boundaries. Then translate those findings into this project's own canonical architecture, contracts, documentation, tests, and implementation.

Never copy source code, packet shapes, schemas, proprietary names, assets, local source paths, or branch-specific quirks into runtime behavior. If a studied system is useful, document the concept first and implement it in our own backend-authoritative model.

### PvP/PK Concept Study

The PvP/PK study retained only the separation between targeting and attack legality, live relation and zone validation, an absolute hostile-exposure deadline refreshed by successful hostile action, death-time PvP-versus-PK classification, durable non-negative consequences, death cleanup before respawn, and server-side peace/safe-area checks at the damage boundary. The hardening pass also treated audit as part of the same authoritative mutation rather than a client or packet concern. For attribution and anti-feed, the useful concepts were limited to assigning the lethal actor at the death boundary and evaluating repeated interactions inside a server-owned time window before any reward policy. This project translates those ideas into its own durable recent-hit assist ledger and investigation-only repeated-pair signal; it does not copy identity checks, reward rules, storage layouts, or implementation mechanics from the reference. Event, duel, siege, olympiad, war, summon attribution, rewards, drops, and advanced karma recovery remain separate future policies. The project-owned contract is recorded in `docs/specs/pvp-pk.md`; no reference code, identifiers, packet layouts, schemas, or assets were copied.

### Session Ownership Concept Study

The session study retained only the lifecycle concepts that one live connection owns a character at a time, connection state advances explicitly before gameplay is accepted, replacement invalidates the previous binding, disconnect first detaches the connection from the character and then performs cleanup, and repeated close or cleanup must be harmless. The reference is process-local, so its implementation was not reused. This project translates those concepts into its own PostgreSQL lease, monotonic fencing token, instance identity, conditional release, early stale-owner reject, and minimal remote-presence lookup. No reference code, class or packet names, storage layout, schema, proprietary identifier, or asset was copied. The project-owned contract is `docs/specs/session-ownership-and-cross-instance-presence.md`.

The cross-instance fanout study retained only the generic responsibilities that recipients come from authoritative presence or visibility, gameplay mutation and outbound delivery are separate responsibilities, entry or exit from presence has explicit delivery consequences, chat text is normalized and bounded by the server, scoped delivery resolves recipients on the server, one unavailable recipient does not invalidate otherwise eligible recipients, invite acceptance revalidates a still-live request, and social mutations notify affected online members. The reference remains process-local and has no reusable multi-instance durability contract. This project therefore created its own PostgreSQL outbox, instance-scoped claim lease, immutable per-command/purpose/recipient idempotency key, retry/dead-letter lifecycle, durable consume receipt, retention policy, exact-session ownership revalidation, remote whisper, exact-recipient region chat, and party/clan state-rehydrating notice consumers. No reference code, schema, packet shape, class name, proprietary identifier, or asset was copied. The project-owned contract is `docs/specs/postgres-gameplay-event-outbox.md`.

## What This Direction Locks In

- The project should feel like a classic fantasy MMORPG with dense, compact spaces.
- The world should avoid giant empty travel and avoid full open-world sprawl.
- The client should feel readable, session-friendly, and spatially grounded.
- The interface should feel like a game client, not like a board game adaptation.

## Spatial Reference Direction

### Compact MMORPG Feel

- Use a city-hub structure with nearby hunting territories.
- Keep space dense enough that players quickly re-enter meaningful play.
- Favor strong landmarks, gates, plazas, bridges, ruins, and obvious pathing.
- Use rocks, walls, ruins, fences, bridges, cliffs, and gates as readable traversal blockers only when server geodata will also treat them that way.
- Favor memorable region identity over sheer map size.

### City Identity

- Each city should have a clear central social or service area.
- Each city should connect to surrounding regions with obvious exits.
- City spaces should feel safe, navigable, and orientation-friendly.

### Territory Identity

- Outer regions should be short, readable, and progression-oriented.
- Enemy zones should signal danger clearly through layout, lighting, palette, and density.
- Terrain should support fast recognition of routes, choke points, and return paths.
- Terrain should make legal alternate routes around blockers visually understandable.

## Art Direction

### Overall Tone

- Use a mysterious, dark-fantasy atmosphere closer to Lineage 2 than to bright arcade fantasy.
- Keep environments grounded in stone, metal, worn wood, muted earth, fog, shadow, and restrained magical accent colors.
- Prefer mood through lighting, silhouettes, spacing, ruins, and color composition rather than through noisy high-frequency texture detail.
- Avoid highly detailed or realistic surface texturing that hurts readability.
- Favor a lowpoly visual language with clean forms and intentional material separation.

### Characters

- Prioritize silhouette readability over realism.
- Keep characters proportionally large enough against the world to preserve the classic MMORPG readability the project is targeting.
- Keep enemy families visually distinct by role and threat.
- Keep base class identity and attribute expectations aligned with canonical Lineage 2 class templates extracted from the studied source.
- Keep player gear progression visually rewarding even in a compact world.
- Equipment must visibly change the player character, especially weapon, chest, helm, gloves, boots, and other major silhouette-driving slots.
- Favor lowpoly character models with clear armor shapes and restrained texture detail instead of flat-looking placeholder geometry.
- Tamed monsters used as pets or mounts should preserve strong species identity while still feeling readable as player-owned companions.

### Class Status Direction

- Base status by playable class should follow the Lineage 2 source we are studying, not a loose homage.
- Treat race, class lineage, template stats, and class-specific baseline identity as canonical inputs for our design.
- If we rebalance later, do it deliberately on top of a documented baseline instead of drifting away implicitly.

### Environment

- Make towns cleaner and more structured than outer regions.
- Make field regions directional and legible rather than maze-like.
- Use landmarks to support memory and navigation.
- Use lighting, fog, elevation, ruins, gates, and negative space to create mystery without sacrificing navigation.
- Keep the world readable at a glance even when the ambience is darker.

### Materials And Textures

- Prefer broad material reads over intricate texture work.
- Use simple tiling, gradients, masks, and stylized wear rather than dense realistic detail maps.
- Let mesh shape, palette, and lighting sell rarity and danger before texture resolution does.
- Reserve stronger visual complexity for bosses, iconic landmarks, and high-value gear tiers.

### Equipment Identity

- Character appearance should be strongly influenced by equipped gear, not just by class base model.
- Gear upgrades should produce visible progression in silhouette, weapon shape, shoulder mass, helm identity, cloak profile, and color accents when applicable.
- Avoid cosmetic systems that leave combat gear visually irrelevant.
- Preserve enough modularity that different equipment combinations still feel like the same character, just more or less advanced.

## UI Direction

### General Feel

- Favor a clean, classic MMORPG HUD with stronger readability than nostalgia clutter.
- Keep the 3D world central and let the HUD support it instead of overpowering it.

### HUD Inspiration

- character status and target information should be always legible
- hotbar and action feedback should be fast to parse
- inventory and equipment should feel game-native, not tabletop-inspired
- minimap, region name, and quest cues should reinforce orientation

### Interaction Feel

- movement, targeting, combat, looting, and NPC interaction should all feel immediate
- town loops should feel calm and efficient
- field loops should feel dense and progression-focused

## Prototype Priorities

1. build one city hub with a readable center
2. build one or two short adjacent combat regions
3. establish player movement, targeting, combat, loot, and NPC interaction
4. establish the classic MMORPG HUD baseline
5. prove that short sessions still feel satisfying
