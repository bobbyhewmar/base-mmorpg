# Interface Architecture

## Direction

Use a hybrid interface:

- `Three.js` for the compact 3D world and world-space feedback
- `HTML/CSS` for the main HUD and text-heavy controls

This keeps the world immersive while preserving fast iteration, responsiveness, and clean UI layout for an MMORPG client.

Read [source-material-reference.md](source-material-reference.md), [world-structure.md](world-structure.md), [combat-and-targeting.md](combat-and-targeting.md), [client-runtime-strategy.md](client-runtime-strategy.md), [specs/server-terrain-geodata-pathfinding.md](specs/server-terrain-geodata-pathfinding.md), [specs/visual-asset-pipeline.md](specs/visual-asset-pipeline.md), [specs/hud-skills-and-hotbars.md](specs/hud-skills-and-hotbars.md), and [specs/hud-inventory-and-classic-windows.md](specs/hud-inventory-and-classic-windows.md) before changing client direction.

## Visual Model

### World View

- Present the world in a high-readability three-quarter view.
- Prefer a constrained elevated camera that supports the compact MMORPG feel.
- Keep movement space, enemy clusters, NPCs, loot, and exits easy to read.
- Preserve strong city-hub identity and short surrounding territories.
- Keep ground-click destinations and targetable mobs visually unambiguous.
- Keep blockers and walkable routes visually aligned with server geodata so pathfinding outcomes feel fair.
- Keep player, NPC, companion, and mob scale as renderer-only presentation constants tuned for Mu Online-like world readability; do not change combat ranges, geodata, or backend authority to fix visual proportions.
- Current player, other-player, and NPC visuals use 50% of the previous character renderer scale. Keep camera target height, orbit distance, zoom limits, and character floating labels aligned with that smaller visual size.
- Camera defaults are intentionally closer than the original large-character camera: the current orbit offset is `(-7.5, 9, 7.5)`, minimum zoom is `3.8`, and maximum zoom is `28`.
- Character locomotion speed is a shared frontend/backend gameplay contract, not a cosmetic renderer value. The current class speeds are `Fighter=3.225` and `Mage=3.075`, with `mireling_strider` mounted speed `4.05`.
- Mob, companion, target-ring, combat range, geodata, and pathfinding remain unchanged by character-only visual rescaling unless explicitly requested and validated separately.
- Loot visuals must stay small and close to the ground. Preserve a larger invisible click hitbox so low-profile drops remain easy to select without making the visible pickup look oversized.
- Mob movement snapshots remain backend-owned, but the Three.js scene must interpolate the visible procedural mob toward the latest authoritative position every frame. Never copy mob positions directly into the visual transform if that reintroduces micro-teleports.
- Procedural mobs need at least simple idle/walk animation on body, head, and legs so pursuit reads as movement instead of sliding chess pieces.
- Player and other-player class body visuals are canonical Universal Base Characters `gltf_base_character` assets loaded from `src/assets/characters/universal-base`; the current creatable Human `Fighter` and `Mage` classes start from the base body for the selected sex and Universal Animation Library clip set.
- Modular outfit assets are not the base class body. They should only become visible later when mapped from authoritative equipped item state.
- Legacy character assets are not the active character source unless a deliberate migration or comparison task says so.
- Do not show procedural humanoids as visual fallback for characters. Until the configured class asset finishes loading, the character should be invisible/transparent; missing class assets are an explicit asset problem, not an invitation to substitute a procedural body.
- Compose modular GLB buildings by repeating original-proportion pieces; do not stretch one wall, roof, or structure mesh non-uniformly into a whole house.
- `Medieval Village MegaKit[Standard]` is available as a source pack in `3DAssets` and is the primary future medieval village/city kit. The active region remains a clean 1024x1024 plain, with only selected ground texture and low vegetation modules published under `src/assets/maps/medieval-village-megakit`; future map modules must be published selectively under `src/assets/maps` and wired through a full map/geodata/spawn concept.
- Retro map assets should use scalar/uniform scale only. If a house, wall, gate, dock, stair, or fence must cover more area, repeat modules at their authored proportion instead of using `[x, y, z]` scale deformation.
- Use dark, mysterious ambience without relying on noisy or ultra-detailed textures.
- Favor lowpoly readability, strong shapes, and lighting-driven mood over realistic material density.

### World-Space UI In Three.js

Keep only spatially meaningful feedback inside the scene:

- selection rings
- target markers
- pending destination markers
- path debug markers only when explicitly enabled for development
- ground-target previews
- area-of-effect previews
- loot markers
- NPC interaction indicators
- region-exit markers
- damage, heal, and status feedback

### Main HUD In HTML/CSS

Keep layout-heavy and text-heavy UI outside the scene:

- player frame and target frame
- pet or companion frame when active
- hotbar and skill cooldowns
- cast-state and selected-skill feedback
- inventory and equipment
- quest tracker only when the quest system needs it; do not show a default task tracker in the combat HUD
- top-right minimap, classic map window, and region name
- chat and system log
- dialogs, vendors, storage, and settings

The HUD shortcut/action surface must include:

- one default horizontal hotbar with `12` slots
- compact `32x32px` slots for skills, inventory items, consumables, or action bindings
- an upward expand control that opens additional `12`-slot rows above the base row
- support for up to `3` visible hotbar rows total
- persistence of bound slot contents and visible hotbar count across disconnect
- shared character-window shortcuts: `ALT+T` Status, `ALT+K` Skills, `ALT+C` Actions, `ALT+N` Clan, and `ALT+U` Quests
- a skills panel opened by `ALT+K` inside the shared character-window family
- top character-window navigation buttons that switch panels and never duplicate learned skill icons
- skill-book categorization between `Active` and `Passive`
- skill-book drag of active icons to hotbar through authoritative `set_hotbar_state` in online mode
- inventory item drag to hotbar through authoritative `set_hotbar_state` in online mode
- `ALT+C` action drag to hotbar through authoritative `set_hotbar_state` in online mode
- a bottom-right classic quick access mini menu with `32x32px` icon buttons for Status, Inventory, Map, and System
- mini-menu shortcuts matching the classic labels: `ALT+T`, `ALT+V`, `ALT+M`, and `ALT+X`
- a top-right minimap driven by player position and scene camera yaw, with player marker and camera-direction arc
- dragged skill or item icon visually follows the cursor until dropped or cancelled
- hover-driven skill tooltips instead of permanently expanded text blocks
- cooldown overlay feedback directly above the icon

The inventory and classic window surface must include:

- inventory closed by default
- `ALT+V` toggling the inventory window
- `ALT+M` opening a classic map window synchronized with the minimap projection and scene camera yaw
- `ALT+X` opening a classic system menu with an explicit Exit Game confirmation modal
- square-corner dark window body with blue title bar
- real clickable close controls
- `32x32px` icon-only item and equipment slots
- hover/focus item tooltips
- footer currency amount and future weight readout

### Pre-Game Screens In HTML/CSS

Keep account and character-entry flows outside the 3D scene:

- login
- registration
- email verification and password recovery feedback
- character list
- character creation
- session-expired and forced-logout messaging

The client may present creation choices, but the backend remains authoritative for:

- whether an account may create another character
- which races are enabled
- which base classes are valid for the chosen race
- whether the chosen sex is valid for the selected template
- whether the chosen name is allowed and unique

The character-creation UI must not invent missing catalog choices. When the catalog provides valid options, the UI preselects the first enabled race and the first class, sex, hairstyle, and skin type option for that template. The current visible catalog is Human-only with `Fighter` and `Mage`; `sex` selects the only available body model for that sex, and `Body Type` must not be exposed until real body variants exist. If the catalog omits a race, class, sex option, or future appearance template, the UI must leave that choice unavailable and block submission until the authoritative source provides it.

## Client Layer Separation

### 1. Scene 3D

Responsibilities:

- render towns, field regions, enemies, NPCs, and drops
- drive camera, movement presentation, and transient effects
- convert terrain clicks into move targets and entity clicks into selected targets
- size the visible terrain and invisible ground raycast/picking plane from the canonical region contract
- start reversible local movement prediction immediately after terrain-click dispatch
- present server-resolved paths and blocked movement feedback
- avoid hardcoded legacy map clamps; terrain clicks may be clamped only to the current region bounds shared with backend geodata
- load local GLB map assets through an explicit renderer manifest instead of ad-hoc external URLs
- treat GLB props as presentation unless backend geodata explicitly declares them as blockers
- make the target lock state unmistakable before hostile skill activation
- visualize combat and interaction feedback
- update the visible character model when equipped gear changes the silhouette or worn pieces

Should not own:

- business truth
- inventory rules
- progression rules
- authoritative combat resolution

### 2. HUD

Responsibilities:

- render player status, target status, hotbar, inventory, and quest information
- render companion or mount status when relevant
- host menus, dialogs, tooltips, and chat
- present confirmations, failures, and interaction context clearly
- host login, registration, character list, and character-creation flows before world entry
- host the skill hotbar stack, skill-book popup, and cooldown readability layer
- host classic inventory/equipment windows using shared visual primitives rather than one-off CSS recreations
- render minimap and map-window projection from runtime view state, not from local map authority

Should not own:

- authoritative state transitions
- hidden combat calculations
- permanent copies of world truth that diverge from the backend
- final validation of account, character-creation, or economy rules
- final truth of learned skills, hotbar legality, or cooldown completion

### 3. Interaction Controller

Responsibilities:

- bridge scene interactions and HUD-driven actions
- manage current target, hover, selected skill, and pending interaction
- dispatch target-point movement, combat, loot, and NPC commands
- send movement destination intent only, never client-supplied waypoints or collision results
- dispatch tame, summon, unsummon, mount, and dismount commands
- dispatch login, registration, character creation, and character entry requests
- dispatch hotbar binding intents through authoritative backend paths when online; current online rebinding uses `set_hotbar_state`
- reconcile UI when authoritative updates invalidate local assumptions

### 4. Client World State

Responsibilities:

- hold the latest authoritative character and nearby world state
- expose derived view state for both scene and HUD
- apply optimistic presentation only when reversible and low-risk
- expose equipment-driven character appearance state cleanly enough for the renderer to swap visible parts without guessing
- keep pre-game account and character-entry state separate from live gameplay state
- hold the latest authoritative learned-skill set, hotbar bindings, and visible-bar configuration for HUD projection

### 5. Platform Bridge

Responsibilities:

- expose runtime-specific capabilities behind internal client-facing contracts
- isolate browser-only or desktop-only integration details
- keep the rest of the client independent from Electron, Tauri, or any other shell

## Interaction Flow

### Entry Flow

1. The player lands on login or registration UI, not directly inside the world.
2. The client submits credentials or registration data to the backend over secure transport.
3. The backend returns account status, verification requirements, the authoritative character list, and the creation catalog.
4. The client shows the classic EULA gate.
5. The client shows the classic server/world-selection gate.
6. The player selects an existing character or opens the creation flow.
7. The player chooses race, base class, sex, hairstyle, skin type, and name.
8. The backend validates the creation request and either rejects it with explicit reasons or persists the character.
9. Only after successful character entry does the client transition into the online world scene.

### Core Loop

1. Player moves through a city or field region in the 3D scene.
2. The scene and HUD surface nearby targets, loot, or NPC interactions.
3. The player clicks terrain to move or clicks a mob to target it.
4. For terrain clicks, the client immediately starts reversible predicted locomotion and may show a pending marker.
5. The backend resolves the route without the client freezing the character in place.
6. The backend returns the accepted route, snapped destination, correction, or rejection.
7. The client blends predicted locomotion into the authoritative route or shows blocked/unreachable feedback.
8. The player sees the selected target clearly in the HUD and world.
9. The player triggers a skill through click context or hotbar input.
10. If the skill requires a hostile target, the client only dispatches it against the current valid target.
11. If the skill needs an area preview, the client shows the target-centered or self-centered shape, and only uses ground targeting for skills that explicitly support it.
12. The client sends the action to the backend.
13. The backend returns accepted state updates or rejection reasons.
14. The client reconciles world state, target state, and cooldown or inventory feedback.

### Skill HUD Flow

1. The player sees one default `12`-slot horizontal hotbar in the HUD.
2. The player may expand additional bars upward, up to `3` visible bars total.
3. The player opens the character-window family with `ALT+T`, `ALT+K`, `ALT+C`, `ALT+N`, or `ALT+U`.
4. The top row switches between Status, Skills, Actions, Clan, and Quests.
5. The Skills panel uses `Active` and `Passive` tabs through compact icon grids.
6. Hovering an icon reveals the skill tooltip with name, attributes, and description.
7. Active skills may be dragged onto hotbar slots, with the icon following the cursor during drag.
8. Inventory items may be dragged onto hotbar slots; equipable item shortcuts call equip, while consumables execute authoritative `use_item`.
9. `ALT+C` actions may become hotbar shortcuts, including `basic_attack` and `pick_up_nearby`.
10. `pick_up_nearby` selects a known loot id but does not perform local pickup retries; the backend owns approach movement, range validation, persistence, and loot disappearance.
11. Online rebinding persists through `set_hotbar_state` before it is treated as durable truth.
12. During cooldown, the icon shows a dark overlay that clears from top to bottom until reuse is available again.

### Design Rules

- Use the 3D scene for movement destination, target choice, ground targeting, and spatial awareness.
- Use the 3D scene for immediate movement prediction and subtle destination feedback, but keep terrain collision authority in the backend. Technical path/geodata lines are debug-only and must stay hidden in normal gameplay.
- Use the HUD for status, hotbar choice, inventory, quests, cast context, and confirmations.
- Keep rejection messaging explicit in the HUD.
- Keep state reconciliation graceful when latency or server authority corrects a local assumption.
- Do not treat offensive skills as free-cast spot actions by default; target lock is the primary combat language.
- Do not let the client invent item prices, total costs, or economy results locally.
- Do not let the client invent learned skills, hotbar persistence, or cooldown completion locally in online mode.
- Do not let the client invent movement paths, obstacle bypasses, or collision legality in online mode.
- Do not let the client invent character appearance defaults in online mode; race, sex, base class, hairstyle, `hair_color`, and `skin_type` must come from the persisted character summary or server presence.

## Initial Game-Screen Layout

### Pre-Game

- full-screen login and registration panels before world entry
- character-selection panel after authentication
- character-creation panel grouped by race, base class, sex, hairstyle, skin type, and name
- character-creation preview renders catalog-backed defaults immediately, then updates as the player changes race, class, sex, hairstyle, or skin type
- character-creation submit is unlocked by a non-empty name once the catalog-backed default template is complete; name availability remains backend-owned
- explicit backend error surfaces for invalid login, unavailable name, invalid race-class combination, and expired session

### Center

- large 3D viewport for the current city or field region
- left mouse button selects targets, picks/interacts where allowed, or sends terrain movement intent
- right mouse button never selects targets and never opens the browser context menu while in the game world
- right mouse drag rotates the camera horizontally and vertically around the player as the fixed orbit pivot
- camera look-at and follow target must be smoothed independently from the player mesh animation, so footstep bob, terrain correction, or authoritative reconciliation do not shake the camera.
- low vertical camera orbit is protected by a ground guard: when the requested orbit would pass below terrain, move the camera closer to the player and keep it above the ground instead of clipping through the map.
- mouse wheel zooms the camera in and out within constrained limits, with a closer minimum distance tuned for the reduced actor scale

### Top Left

- player frame
- target frame
- short-lived status effects

### Top Right

- minimap
- current region or city name
- quest tracker or activity hints only when backed by an actual quest/activity system

### Bottom Center

- primary hotbar row with `12` horizontal `32x32px` slots
- upward expand control for up to `3` stacked rows
- cooldown visibility
- skill or item feedback
- cast mode and selected-skill feedback
- mount-state feedback when mounted

### Bottom Left

- chat
- system log
- optional social notifications

### Right Side

- inventory, equipment, character sheet, and quest details in toggleable classic panels
- inventory opens with `ALT+V` and is closed by default

### Modal Layer

- vendor interactions
- storage
- quest dialogs
- settings
- party or social prompts
- skill-book popup opened by `ALT+K`

## Responsiveness

- Preserve the 3D viewport as the primary focus on desktop.
- Keep the canonical interaction path viable in the browser without requiring a desktop shell.
- Collapse secondary panels into drawers or tabs on smaller screens.
- Keep combat-critical HUD elements visible without covering the player area excessively.

## Testing Implications

- Test HUD logic separately from the scene renderer when possible.
- Test click-to-move, click-to-target, area previews, looting, and NPC interaction as contracts between scene, HUD, and backend.
- Test that right-click does not select targets, does not move the player, and does not open browser context menu.
- Test that right-drag rotates the camera around the player on both orbit axes, low pitch remains above the ground, and wheel zoom stays bounded.
- Test that the local player begins movement immediately after terrain click and later reconciles to the server path.
- Test that blocked or unreachable movement produces clear UI state and does not desync from the server route.
- Test that authoritative updates can replace stale local assumptions without leaving broken UI state.
