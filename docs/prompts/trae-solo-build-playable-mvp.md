# Trae SOLO Prompt To Build The First Playable MVP

Use this prompt when we want Trae SOLO to build the first playable browser MVP from this project's current documentation.

## Goal

Build a local playable vertical slice that proves the compact MMORPG feel before online persistence, account systems, WebSockets, or production backend work.

This MVP should be a real playable game screen, not a landing page, tech demo, or static mockup.

## Prompt To Copy

```text
You are working inside this project's repository. Build the first playable MVP for the compact MMORPG described by the local docs.

OBJECTIVE:
Create a browser-playable local MVP that proves the core loop:
- enter a compact city hub
- move with click-to-move
- leave into a nearby hunting field
- click a mob to target it
- use target-locked skills
- kill enemies
- receive loot
- open inventory/equipment
- equip an item and see the character visual change
- return to town and interact with an NPC

IMPORTANT DIRECTION:
- Use the source-study docs as concept input, not as a porting target.
- Build our own implementation, assets, names, data structures, and UI.
- Do not copy Lineage 2 code, schemas, packet shapes, enum coupling, or exact content.
- Keep the build intentionally small but actually playable.
- Prefer a browser-first local prototype over an online backend for this first version.
- Model local game commands through a small internal domain layer so it can evolve into an authoritative backend later.

READ THESE DOCS FIRST:
- docs/engineering-principles.md
- docs/delivery-roadmap.md
- docs/architecture-overview.md
- docs/interface-architecture.md
- docs/combat-and-targeting.md
- docs/world-structure.md
- docs/source-material-reference.md
- docs/domain-and-data.md
- docs/interlude-source-study/02-combat-stats-skills-and-pve.md
- docs/interlude-source-study/03-progression-classes-quests-and-items.md
- docs/interlude-source-study/06-class-template-and-stat-baseline.md
- docs/interlude-source-study/07-inventory-equipment-and-item-usage.md
- skills/threejs-client-engineer/references/client-implementation-guidelines.md

TECHNICAL DEFAULTS:
- If no frontend app exists, scaffold a Vite + TypeScript app.
- Use Three.js for the world scene.
- Use HTML/CSS for the HUD.
- Keep the 3D scene full-bleed or unframed as the primary screen.
- Keep state in local TypeScript modules for now.
- Keep gameplay rules separate from Three.js rendering and HUD code.
- Use deterministic local domain functions for movement, targeting, combat, loot, inventory, and equipment.
- Do not introduce a production backend, database, auth, Redis, queues, or email integration in this MVP.
- Do not add heavy frameworks unless the repo already uses them.

REQUIRED GAMEPLAY:

1. World
- One compact city hub.
- One adjacent low-level hunting field.
- One slightly deeper field area or danger pocket.
- Clear visual boundaries for safe town versus combat area.
- A few landmarks such as gate, plaza, ruins, merchant stand, or road.
- Dark mysterious lowpoly art direction with readable silhouettes.

2. Camera And Movement
- Elevated three-quarter camera inspired by classic compact MMORPGs.
- Click terrain to move.
- Show a visible ground destination marker.
- Movement must feel responsive and readable.
- No WASD as the primary movement model.

3. Targeting
- Click a mob to set current target.
- Show selected target ring in the 3D scene.
- Show target frame in the HUD.
- Optional tab target is allowed if simple, but click-target is primary.

4. Combat
- Hostile skills require a valid current target.
- Implement at least:
  - one basic single-target attack
  - one slower target-centered AoE with split damage
- Show cooldowns, cast feedback, damage numbers, and death feedback.
- Prevent free AoE spam into empty spots.
- Enemies can have simple local AI: idle, aggro when near or attacked, basic melee attack, death, respawn.

5. Character And Stats
- Implement one starting class or archetype using our own naming.
- Include HP, MP, level, XP, basic attack, defense, move speed, and simple derived stats.
- Use the class/source-study docs for conceptual separation only:
  - class identity
  - static template seeds
  - growth values
  - current HP/MP
  - derived runtime values
- Do not reproduce exact Lineage class ids or formulas.

6. Loot And Inventory
- Defeated enemies can drop at least one currency-like item and one equipment item.
- Loot appears in world and can be clicked.
- Inventory is an HTML/CSS HUD panel.
- Item template and item instance should be separate in code.
- Inventory state should track instance id, template id, quantity, container, and equip slot when relevant.
- Support at least one stackable item and one equippable weapon or armor item.

7. Equipment And Visual Change
- Equipment slots should be visible in the HUD.
- Equipping an item should update stats.
- Equipping an item should visibly change the character model in Three.js.
- It is acceptable to use lowpoly primitive gear parts for the MVP, but the visual change must be obvious.

8. NPC And Town Loop
- Add at least one town NPC.
- NPC interaction opens a simple HTML dialog.
- NPC can explain or trigger a simple task such as "defeat 3 enemies" or "bring back a field drop".
- Keep this as an in-game interaction, not tutorial copy on the page.

9. HUD
- HTML/CSS overlay, not inside Three.js.
- Must include:
  - player frame
  - target frame
  - hotbar
  - inventory/equipment panel
  - small quest/task tracker
  - system log
  - current region name
- The first screen should be the playable game UI, not a marketing page.

10. Persistence For MVP
- Use localStorage only for optional local save/load of the prototype state.
- No PostgreSQL yet.
- Keep save/load isolated behind a small adapter so it can be replaced later.

REQUIRED ARCHITECTURE:

- Separate modules roughly by responsibility:
  - game data/templates
  - local domain state
  - command handlers
  - movement and combat rules
  - inventory/equipment rules
  - Three.js scene/rendering
  - HUD/UI
  - persistence adapter
- Keep domain rules testable without Three.js.
- Keep rendering derived from state, not the source of truth.
- Make commands explicit:
  - moveToPoint
  - selectTarget
  - useSkill
  - pickUpLoot
  - equipItem
  - unequipItem
  - interactNpc

ACCEPTANCE CRITERIA:

- The project installs and runs with documented commands.
- A user can play the loop from city to field to combat to loot to inventory to equipment change.
- The target-locked combat rule is enforced.
- The AoE skill requires a target and distributes damage across affected enemies.
- Inventory distinguishes item templates from item instances.
- Equipping gear changes both stats and visible character parts.
- The HUD remains readable on desktop and a reasonable mobile viewport.
- The scene is nonblank, interactive, and framed correctly.
- No unrelated systems are added.

VERIFICATION:

- Run install/build/typecheck commands available in the project.
- Start the dev server.
- Verify manually in browser or with Playwright if available:
  - desktop viewport
  - mobile viewport
  - scene renders
  - click-to-move works
  - target selection works
  - skills work
  - loot pickup works by direct drop click and nearby-pickup action without client-side retry authority
  - inventory and equipment work
  - visible gear changes render
- Report the dev server URL and any limitations.

DELIVERABLES:

- Working MVP code.
- Short README section or docs note explaining:
  - how to run it
  - what is included
  - what is intentionally deferred
- Keep comments concise and useful.

EXPLICITLY DEFER:

- login/account system
- real backend
- PostgreSQL migrations
- WebSocket networking
- clans, party, PvP, castle siege, olympiad
- taming/pets/mounts
- warehouses and trade
- production observability
- transactional email
- asset pipeline for final 3D models
```

## Why This Scope

This is a Phase 1 local world prototype. It proves the feel of the game before we commit to online architecture work.

The Online MVP comes after this once movement, camera, targeting, combat, loot, inventory, and equipment are fun enough to keep.
