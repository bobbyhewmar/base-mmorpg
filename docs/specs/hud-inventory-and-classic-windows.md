# HUD Inventory and Classic Windows

## Objective

Define the canonical visual and interaction rules for classic HUD windows, especially inventory and equipment.

This spec exists to prevent future UI slices from recreating rounded, card-like, or modern dashboard panels when the project direction is a compact classic MMORPG HUD.

## Current Implemented Slice

The current client already implements:

- direct login-first pre-game flow instead of asking the player to choose online or local mode on first load
- Lineage-like dark forest login background without Lineage branding
- shared classic button styling between login and register flows
- player status frame with level, character name, and CP/HP/MP bars
- target frame centered near the top when a mob, NPC, or player is selected
- bottom-left chat/system log
- top-right minimap with player-centered map projection and camera-direction arc
- bottom-center hotbar with compact `32x32px` slots
- shared character-window navigation with `ALT+T` Status, `ALT+K` Skills, `ALT+C` Actions, `ALT+N` Clan, and `ALT+U` Quests
- character windows using the same compact top navigation buttons instead of duplicating skills above the `Active` and `Passive` tabs
- inventory closed by default
- inventory toggle through `ALT+V`
- inventory close button in the window title bar
- inventory/equipment as a square-corner classic window with dark body and blue title bar
- icon-only item grid with item details shown on hover or focus
- coin amount displayed in the inventory footer so the player does not need to search for the currency icon
- future weight readout placeholder in the inventory footer
- draggable classic windows through their blue title bars or drag rails where implemented
- explicit skill drag affordance: when an active skill is dragged, its icon follows the cursor until dropped on a hotbar slot or cancelled
- inventory item drag affordance: when an item is dragged to the shortcut bar, its icon follows the cursor until dropped or cancelled

## Visual Contract

Classic HUD windows must use:

- square corners
- dark translucent or near-black body
- blue gradient title bar
- small square title controls
- thin borders and inset lines
- compact `32x32px` icon grids for items and skills
- shared `32x32px` icon navigation buttons for the character-window family
- hover/focus tooltips for details instead of permanently expanded item cards

Classic HUD windows must not use:

- rounded cards
- oversized `64x64px` skill or inventory cells
- large item text cards inside the inventory grid
- modern dashboard spacing
- decorative close icons that do not click
- task tracker panels in the default combat HUD
- duplicated skill shortcut rows above skill categories

## Bottom-Right Quick Access Mini Menu

Status: implemented in the client HUD.

The gameplay HUD must add a bottom-right quick access mini menu matching the classic client reference.

Required visual rules:

- fixed in the bottom-right corner, close to the screen edge but not overlapping the bottom-center hotbar
- one narrow vertical blue drag rail on the left side of the mini menu
- four compact `32x32px` icon buttons in a single horizontal strip
- square corners, dark body, thin borders, and the same blue/inset highlight language used by the hotbar and classic windows
- tooltip above the hovered button using the exact shortcut label style, such as `Character Status (Alt+T)`, `Inventory (Alt+V)`, `Map (Alt+M)`, and `System (Alt+X)`

Required buttons for the first slice:

- `Character Status`: opens the existing `ALT+T` status window
- `Inventory`: opens/toggles the existing `ALT+V` inventory window
- `Map`: opens a classic map window placeholder through `ALT+M`
- `System`: opens a classic system menu through `ALT+X`

The system menu must be a classic square window above the mini menu and include icon rows for future features such as Community, Macro, Help, Petition, Options, Restart, and Exit Game. Only implemented actions may mutate state. Unimplemented rows may be disabled or informational, but they must not fake success.

`Exit Game` opens a classic confirmation modal with warning icon, message `Do you wish to exit the game?`, and `OK` plus `Cancel` buttons. `Cancel` closes only the modal. `OK` currently performs an explicit client reload to return to the pre-game flow until a fuller logout/session-teardown endpoint exists.

## Minimap And Map Window

Status: implemented as a first synchronized HUD slice.

The top-right minimap and the `ALT+M` map window must use the same presentation source:

- player position from the current `GameState`
- current camera yaw exposed by the Three.js scene
- canonical `1024x1024` region projection for the current clean plain map

The minimap is fixed in the top-right corner and is not draggable. It keeps the player marker visually centered, shifts the illustrative map underneath it, and renders a small camera-direction arc around the center marker.

The `ALT+M` map window remains a classic draggable window, but it must not be static. It uses the same player position and camera yaw as the minimap, renders the current player marker in map coordinates, and rotates the camera-direction arc with the scene camera.

Until a full authored map-texture pipeline exists, both surfaces may use a procedural illustrative map. They must not invent gameplay authority, hidden locations, or map blockers; they are navigation presentation only.

## Inventory Rules

### Access

- Inventory is closed by default.
- `ALT+V` toggles inventory.
- The title-bar close button closes inventory.
- The inventory window can be dragged from the blue title bar.

### Layout

Inventory contains:

- a title bar labeled `Inventory`
- an equipment area with compact slots
- `Item` and `Quest` tabs
- an item grid using `32x32px` slots
- a footer with currency amount, future weight percentage, and discard/trash affordance

### Items

- Items render as icons only in the grid.
- Item name, type, quantity, attributes, and available actions appear on hover/focus.
- Equip, unequip, split, merge, vendor, warehouse, and trade actions remain UI affordances over backend-authoritative commands.
- Inventory items can be dragged to the bottom shortcut/action bar.
- Equipable item shortcuts execute the same equip flow as clicking Equip in inventory.
- Consumable item shortcuts execute through the authoritative `use_item` command before any gameplay mutation is shown as truth.
- The client must never derive final economy value, equip legality, or inventory mutation truth locally.

### Currency And Weight

- Currency must have both an inventory item representation and a footer readout.
- Footer readout is the fast way to read currency amount.
- Weight percentage is reserved for future authoritative inventory weight/load rules and may remain a placeholder until that system exists.

## Player Status Frame

The player frame should stay compact and close to the original classic MMORPG structure:

- level and player name near the top
- CP, HP, and MP labels next to compact bars
- current/max values rendered over each bar
- XP percentage only when available

Do not reintroduce debug movement messages, ATK/DEF/MOVE readouts, or task-tracker content into the default player frame.

## Window Controls

- Close controls must be real `button` elements or equivalent clickable controls with explicit handlers.
- Decorative title controls may exist only if they are intentionally non-functional and cannot be confused with a working control.
- If a control looks clickable, it must either work or be removed.

## Character Window Family

- `ALT+T` opens Status and attributes.
- `ALT+K` opens Skills with `Active` and `Passive` tabs.
- `ALT+C` opens Shortcuts and Actions.
- `ALT+N` opens Clan.
- `ALT+U` opens Quests and missions.
- These panels share the same title bar, square body, close behavior, and top navigation row.
- The navigation row is for switching panels only; it must not be used as a learned-skill shortcut strip.

## Authority Rules

- HUD windows are presentation and interaction surfaces only.
- Backend remains authoritative for inventory, equipment, item ownership, item quantities, currency, economy values, and progression.
- Client-local window position, open/closed state, hover state, and temporary drag state are allowed as presentation state.

## Acceptance Criteria

- Opening inventory with `ALT+V` shows a compact classic window, not a card dashboard.
- Clicking the inventory close button closes the window.
- Items render as `32x32px` icons.
- Item details appear through hover/focus tooltip behavior.
- Currency can be read from the footer.
- The default HUD has no task tracker panel.
- The visual language matches the skill-book and hotbar specs.
