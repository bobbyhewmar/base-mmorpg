# Client Implementation Guidelines

Read [../../../docs/interface-architecture.md](../../../docs/interface-architecture.md), [../../../docs/combat-and-targeting.md](../../../docs/combat-and-targeting.md), [../../../docs/client-runtime-strategy.md](../../../docs/client-runtime-strategy.md), [../../../docs/specs/server-terrain-geodata-pathfinding.md](../../../docs/specs/server-terrain-geodata-pathfinding.md), [../../../docs/specs/hud-skills-and-hotbars.md](../../../docs/specs/hud-skills-and-hotbars.md), [../../../docs/specs/hud-inventory-and-classic-windows.md](../../../docs/specs/hud-inventory-and-classic-windows.md), [../../../docs/source-material-reference.md](../../../docs/source-material-reference.md), and [../../../docs/world-structure.md](../../../docs/world-structure.md) before defining a new client flow.

## Scene Structure

- Separate world geometry, characters, enemies, effects, and interaction helpers into clear modules.
- Keep camera and input logic independent from domain state transforms.
- Represent authoritative entities with stable identifiers so updates can patch the scene predictably.
- Prefer a constrained elevated camera for stable movement and target readability.
- Use left click for gameplay selection/movement only; use right-drag for horizontal camera orbit around the player pivot.
- Suppress the browser context menu while the game world is active.
- Use mouse wheel for bounded camera zoom and avoid changing pitch during basic orbit.
- Treat characters, enemies, NPCs, movement previews, and target indicators as world-space concerns.
- Preserve clear region reads: landmarks, exits, safe hubs, and danger gradients.
- Make ground-click destinations, selected targets, and AoE previews visually distinct at a glance.
- Make immediate predicted locomotion, pending movement paths, authoritative movement paths, and blocked destinations visually distinct.
- Use smooth reconciliation from predicted movement to authoritative server path under normal latency.
- Keep visible blockers aligned with server geodata assumptions.
- Favor lowpoly geometry, disciplined silhouettes, and atmosphere from lighting instead of heavy texture detail.
- Build the player character renderer so visible gear changes can swap weapons and major armor pieces without replacing the whole character pipeline.
- Render companions and mounts as first-class scene actors with readable ownership and target cues.

## HUD Structure

- Keep the main HUD in HTML/CSS, not inside the Three.js scene.
- Use the HUD for inventory, stats, action buttons, logs, dialogs, and text-heavy information.
- Keep scene overlays in Three.js only when they are spatially meaningful.
- Keep the HUD readable in the style of a classic compact MMORPG client.
- Keep classic HUD windows square, dense, and consistent: blue title bars, dark bodies, `32x32px` icon slots, hover/focus tooltips, and working title controls.
- Reuse one character-window system for `ALT+T` Status, `ALT+K` Skills, `ALT+C` Actions, `ALT+N` Clan, and `ALT+U` Quests.
- Reserve the row above `Active` and `Passive` for character-window navigation buttons only; do not duplicate learned skills there.
- Treat the bottom bar as a shortcut/action bar for skills, items, consumables, and explicit actions, not as a skill-only surface.
- Show active skill and inventory item drag with a `32x32px` icon ghost following the cursor until the player drops or cancels it.
- Support clearing occupied shortcut/action slots by dragging them out of the bar or using `ALT + left click`.
- Keep consumable use and `ALT+C` actions, including `basic_attack`, behind explicit authoritative command contracts instead of hidden local fallbacks.
- Shape combat, inventory, and vendor flows around speed and clarity, not nostalgia clutter.
- Do not default to task trackers, rounded cards, permanently expanded inventory item cards, or `64x64px` HUD grids.
- Keep the target frame, selected skill, and cast mode unmistakable during combat.
- Keep companion status, pet commands, and mount restrictions readable without crowding the primary combat HUD.
- Make equipment slots, item state, storage context, and invalid-use feedback clear without forcing the player to infer hidden rules.

## Runtime Boundary

- Treat the browser as the primary runtime target.
- Keep desktop-specific features behind a platform bridge.
- Do not let Electron, Tauri, or browser-only APIs leak across the client architecture by default.

## State Flow

- Consume authoritative character and nearby-world updates from the backend.
- Keep optimistic UI limited and reversible.
- Treat local movement prediction as reversible presentation until the authoritative route arrives.
- Keep local prediction bounded by a leash so the actor does not run far past server truth.
- Derive transient animations from state changes instead of mutating permanent truth in the client.
- Route selection state through a controller that both scene and HUD can consume cleanly.
- Treat ground-targeted casts and entity-targeted casts as separate client states, not one overloaded mode.
- Keep companion selection, active summon state, and mounted presentation derived from authoritative state rather than hidden client toggles.

## Performance Checklist

- Measure frame time before optimizing.
- Reuse geometry and materials when possible.
- Avoid unnecessary rerenders of overlay UI.
- Treat asset size and load order as first-class UX concerns.
- Prefer modular gear parts, shared rigs, and reusable materials over bespoke full-character variants for every item combination.

## UX Checklist

- Make selectable actions obvious.
- Make server rejections understandable.
- Make blocked, snapped, and unreachable movement outcomes understandable.
- Make click-to-move feel immediate even when the server is still resolving the route.
- Make the current player, target, cooldown, and quest context legible at a glance.
- Use the 3D scene for movement and target choice and the HUD for skill, inventory, and confirmation flows.
- Make skill/item-to-hotbar drag readable before commit by keeping the dragged icon visibly attached to the cursor.
- Keep the interface feeling like a compact MMORPG client instead of a sprawling or cluttered one.
- Make AoE radius, multi-target pickup, and invalid target situations obvious before and after cast.
- Make equipment upgrades visibly satisfying on the live character model, especially for weapon and major armor-slot changes.
- Make tameable monsters, active pets, and mounted restrictions obvious before the player commits to those actions.
- Make equip, unequip, storage transfer, split-stack, and blocked-item-use outcomes obvious in the HUD state and action feedback.
- Never hide a failed server pathfinding result behind a local fallback route.
- Never make the local player wait motionless for pathfinding in normal latency.
