---
name: threejs-client-engineer
description: Build and critique the Three.js client, compact world scene structure, and client-side UX for the compact MMORPG project. Use when Codex needs to design rendering architecture, manage client state, synchronize authoritative character and world updates, or implement movement, targeting, combat, companion, mount, and world presentation.
---

# Three.js Client Engineer

Read [../../docs/architecture-overview.md](../../docs/architecture-overview.md), [../../docs/interface-architecture.md](../../docs/interface-architecture.md), [../../docs/combat-and-targeting.md](../../docs/combat-and-targeting.md), [../../docs/client-runtime-strategy.md](../../docs/client-runtime-strategy.md), [../../docs/specs/server-terrain-geodata-pathfinding.md](../../docs/specs/server-terrain-geodata-pathfinding.md), [../../docs/specs/hud-skills-and-hotbars.md](../../docs/specs/hud-skills-and-hotbars.md), [../../docs/specs/hud-inventory-and-classic-windows.md](../../docs/specs/hud-inventory-and-classic-windows.md), [../../docs/source-material-reference.md](../../docs/source-material-reference.md), [../../docs/world-structure.md](../../docs/world-structure.md), [../../docs/domain-and-data.md](../../docs/domain-and-data.md), and [../../docs/engineering-principles.md](../../docs/engineering-principles.md) before changing client architecture.
Read [../../docs/interlude-source-study/README.md](../../docs/interlude-source-study/README.md) and [../../docs/interlude-source-study/01-character-world-and-lifecycle.md](../../docs/interlude-source-study/01-character-world-and-lifecycle.md) when translating classic Lineage-like interaction patterns into the browser client.

## Workflow

1. Define the user interaction and the authoritative state it depends on.
2. Separate render concerns from gameplay state concerns.
3. Keep scene structure, camera behavior, animation, and UI responsibilities explicit.
4. Design client updates to tolerate delayed or corrected server state.
5. Produce implementation guidance that another agent can turn into code without guessing.

## Default Stance

- Keep the client presentational first and authoritative never.
- Prefer a hybrid interface: Three.js for the world and HTML/CSS for the main HUD.
- Prefer the browser as the canonical runtime and keep desktop packaging optional.
- Prefer a constrained elevated camera that preserves readability in compact regions.
- Keep camera controls explicit: left click is gameplay interaction, right-drag is camera orbit around the player, and wheel is bounded zoom.
- Never let right-click select targets, move the character, or open the browser context menu while the game world is active.
- Keep local state limited to interaction, animation, and temporary UI flows.
- Prefer deliberate scene graph organization over ad hoc object mutation.
- Optimize only after measuring frame time, bundle size, or asset cost.
- Preserve the compact MMORPG identity instead of drifting into generic open-world spectacle or cluttered UI nostalgia.
- Keep runtime-specific APIs behind an internal platform bridge.
- Make terrain click, target click, and area previews visually and behaviorally distinct.
- Start local movement prediction immediately after terrain-click dispatch and replace or blend it with server-resolved routes; do not create client-side pathfinding authority.
- Use a prediction leash so responsiveness does not become long-distance client-side truth.
- Keep visual blockers aligned with server geodata so blocked routes feel fair.
- Keep classic HUD windows consistent: square corners, blue title bars, compact `32x32px` slots, icon-only grids, hover/focus tooltips, and real clickable controls.
- Treat the character-window family as one reusable HUD system: `ALT+T` Status, `ALT+K` Skills, `ALT+C` Actions, `ALT+N` Clan, and `ALT+U` Quests share the same top navigation row.
- Never use the top row above `Active` and `Passive` as a duplicated learned-skill strip; that row is only for switching character panels.
- Treat the bottom bar as a shortcut/action bar, not a skill-only bar.
- Support shortcut semantics for active skills, inventory items, consumables, and explicit actions while keeping backend authority for legality.
- When dragging an active skill or inventory item to the hotbar, keep a visible icon ghost attached to the cursor until drop or cancel.
- Let occupied shortcut/action bar slots be cleared by dragging out of the bar or by `ALT + left click`; keep this local until backend persistence exists.
- Do not implement consumable use or `basic_attack` as hidden local fallbacks; require authoritative `use_item` and action commands.
- Do not reintroduce default task trackers, rounded HUD cards, or oversized `64x64px` action/inventory grids.
- Make active companions, tameable monsters, and mounted state visually clear without confusing them with the player's primary target flow.

## Deliverables

- scene architecture proposal
- HUD architecture proposal
- selection and action-flow proposal
- interaction flow for movement, targeting, combat, loot, and NPC interaction
- click-to-move and skill-targeting proposal
- immediate movement prediction, pending path, authoritative path, reconciliation, and blocked-movement presentation proposal when movement UX is touched
- companion or mount interaction proposal covering tame, summon, pet presence, and mounted HUD feedback
- platform-bridge proposal for runtime-specific capabilities
- visual adaptation notes that map world direction into digital surfaces and HUD
- state synchronization notes
- asset-loading or rendering risk list
- implementation plan for UI and 3D modules

## Coordination

- Pull in `systems-designer` when visual interaction implies rule ambiguity.
- Pull in `backend-gameplay-engineer` when client flows need new command or event contracts.
- Pull in `qa-automation-engineer` when interaction states need regression coverage.

## References

Read [references/client-implementation-guidelines.md](references/client-implementation-guidelines.md) when implementing or reviewing client work.
