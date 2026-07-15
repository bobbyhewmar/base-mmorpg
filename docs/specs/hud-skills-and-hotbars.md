# HUD Skills and Hotbars

## Objective

Define the authoritative HUD rules for skill presentation, shortcut/action hotbars, skill-book access, cooldown visuals, item/action bindings, and persistent hotbar layout.

This document freezes:

- the primary shortcut/action hotbar structure
- slot count and slot size
- stacked-bar expansion behavior
- the shared character-window navigation triggered by `ALT+T`, `ALT+K`, `ALT+C`, `ALT+N`, and `ALT+U`
- the skill panel triggered by `ALT+K`
- the distinction between active and passive skills
- the classic compact MMORPG visual contract for `32x32px` icon slots
- tooltip and cooldown presentation rules
- persistence expectations for hotbar layout, bound entries, and open-bar count

## Current Implemented Slice

The current repository implementation already ships:

- learned skills derived authoritatively from base class plus level
- active and passive separation in the skill book
- persistent backend-owned hotbar rows and slot bindings
- hotbar rendering from authoritative slot data rather than a global hardcoded skill list
- classic bottom-center hotbar with `32x32px` slots, one blue drag rail on the left, and a `16x16px` expand button on the right
- hotbar rows expanding upward, from bottom to top
- shared character window opened through `ALT+T` Status, `ALT+K` Skills, `ALT+C` Actions, `ALT+N` Clan, and `ALT+U` Quests
- skill panel visual refactor to a Lineage-like rectangular panel with a blue title bar, shared top panel-navigation buttons, `Active` and `Passive` tabs, and dense icon-only grid slots
- active skill icons can be dragged from the skill book to hotbar slots and persisted online through `set_hotbar_state`
- inventory items can be dragged to hotbar slots and persisted online through `set_hotbar_state`
- `ALT+C` actions such as `basic_attack` and `pick_up_nearby` can be dragged to hotbar slots and persisted online through `set_hotbar_state`
- `ESC` clears the selected target and target-driven queued actions through the authoritative `clear_target` command when chat input and NPC dialogs are not consuming the key
- equipable item shortcuts call the same equip flow as the inventory when clicked
- consumable item shortcuts execute through the authoritative `use_item` command and must not be faked locally
- `ALT+C` action shortcut bindings are durable, while their gameplay execution still uses the action's own authoritative command path
- `pick_up_nearby` does not require a target; it chooses the nearest known loot, sends one authoritative `pick_up_loot`, and lets the backend queue approach plus collection without a client retry loop
- active skill drag shows the skill icon attached to the cursor until it is dropped or cancelled
- cooldown overlay rendered directly on the icon with top-to-bottom clearing
- `SYNC` cooldown state shown after the server-provided cooldown end timestamp elapses locally but before a fresh cooldown snapshot arrives

The current online slice persists HUD-driven rebinding through the authoritative `set_hotbar_state` gameplay command. The command stores a full `36`-slot hotbar snapshot in `character_hotbar_loadouts`, including `open_bar_count`, empty slots, `skill`, `item`, and `action` entries. Reconnect, `world/enter`, and runtime deltas rehydrate the same authoritative projection.

## Decision

The main gameplay HUD must expose a horizontal shortcut/action bar at the bottom-center of the screen.

The default visible bar contains:

- `12` slots
- each slot sized at `32x32px`
- one slot immediately beside the next in a single horizontal row

Each slot may hold:

- an active skill
- an inventory item shortcut
- an equipable item shortcut that equips the item when clicked
- a consumable item shortcut that consumes the item through an authoritative `use_item` command
- an action shortcut explicitly supported by the game rules, such as `basic_attack`

The system must support up to `3` stacked hotbars total.

The first bar is the default base row.

Additional bars open above the base row, never below it.

## Main Hotbar Layout

### Default Bar

- Always render one primary bar in the HUD.
- Render `12` horizontal slots in one row.
- Keep slot spacing visually tight enough to read as one action strip, not separate floating buttons.
- Use the bar as the primary location for active skill, item, consumable, and action activation feedback.
- Keep the hotbar visually minimal: grid slots plus one blue vertical drag rail on the left.
- The right side is reserved for the small expand button, not another drag rail.

### Expanded Bars

- Place an expand control with an upward arrow next to the base bar.
- Use a small square control, approximately `16x16px`, aligned to the right side of the bar.
- Clicking the arrow opens one additional `12`-slot row above the currently highest visible row.
- The player may open up to `3` total bars:
  - bar 1: default row
  - bar 2: appears above bar 1
  - bar 3: appears above bar 2
- New bars must always stack upward from the default row.
- Collapse behavior may reuse the same control later, but expansion ordering must remain bottom-to-top.

### Slot Semantics

- A slot is a binding location, not a source of gameplay authority.
- Slot content is a projection of backend-authoritative character loadout and allowed actions.
- The client may render drag, click, or assignment affordances, but the backend remains authoritative for whether the bound skill, item, consumable, or action is valid for that character.

## Persistence

- The bound contents of each visible hotbar slot must persist across disconnect and reconnect.
- The number of currently open hotbars must also persist across disconnect and reconnect.
- In online play, this persistence is authoritative on the backend.
- The client may cache layout locally for UX continuity, but local cache is never the final truth.

The minimum persisted hotbar state per character should express:

- open bar count
- slot index
- bound entry type
- bound skill id, item instance id, consumable item instance id, or action id
- optional future shortcut metadata

Valid slot entry types:

- `skill`: active learned skill id
- `item`: item instance id, usually equipable or consumable
- `action`: explicit action id such as `basic_attack`

Supported action bindings currently include:

- `basic_attack`: uses the selected living hostile target and lets the backend move into range before attacking when needed.
- `pick_up_nearby`: finds the nearest known loot client-side only to select the entity id, then delegates range, movement, persistence, and disappear fan-out to the backend.

## Character Windows And Skill Panel

### Access

- The player opens the shared character window with these shortcuts:
- `ALT+T`: Status and character attributes.
- `ALT+K`: Skills.
- `ALT+C`: Shortcuts and actions.
- `ALT+N`: Clan panel.
- `ALT+U`: Quests and missions.
- The skills view is an HTML or HUD-layer popup, not an in-world diegetic panel.
- The popup should appear as a vertical rectangular panel.
- The panel should use the same square-corner, dark body, blue title-bar language as the inventory and other classic HUD windows.
- The title bar must not be wider than the body.
- The close control must be a real clickable button, not a decorative icon.
- The row directly under the title bar is reserved for the five character-window navigation buttons.
- Do not render learned skill shortcuts in the navigation row.

### Grid Layout

- Render skill icons in a grid using `32x32px` slots.
- Prefer `6` columns for the current compact skill-book body.
- Keep icon density high enough to browse many learned skills without turning the panel into a full-screen window by default.
- Keep skill names and details out of the grid body; icon hover or focus reveals details.
- Keep the top navigation row as icon-only `32x32px` panel buttons for Status, Skills, Actions, Clan, and Quests.

### Categories

The popup must separate skills into:

- `Active`
- `Passive`

#### Active Skills

Treat as active every skill that performs an explicit action when triggered, including:

- combat skills
- buffs
- debuffs
- other triggered skills that the player intentionally activates

#### Passive Skills

Treat as passive every skill that modifies the character through persistent attribute or rule changes without direct activation, including:

- direct stat modifiers
- passive combat bonuses
- passive defensive or utility modifiers

## Icon and Tooltip Rules

- Every skill must have an icon.
- The icon is the primary visual representation both in hotbars and in the skill book.
- Skill details must not stay permanently expanded in the skill book grid.
- Show skill details only on hover over the skill icon.

The hover tooltip must contain at least:

- skill name
- skill attributes or key effects
- skill description

## Cooldown Visual Rule

- Cooldown must be shown directly on the skill icon.
- The cooldown visual should use a shadow or dark overlay layer above the icon.
- When the skill enters cooldown, the overlay should visibly cover the icon.
- As the skill approaches availability again, the overlay must recede from top to bottom, progressively clearing the icon.
- The icon should feel fully readable again exactly when the cooldown ends.

This is a readability rule, not merely a decorative animation.

## Interaction Rules

- The hotbar is the primary launch surface for active skills already bound to slots.
- The skill book is the browsing surface for learned skills.
- Active skills may be dragged from the skill-book icon grid to hotbar slots.
- While dragging a skill, the cursor must visibly carry the skill icon until drop or cancel.
- Inventory items may be dragged from the inventory icon grid to hotbar slots.
- While dragging an item, the cursor must visibly carry the item icon until drop or cancel.
- Actions may be dragged from `ALT+C` to hotbar slots.
- Existing hotbar shortcuts may be dragged out of the hotbar to clear their slot.
- `ALT + left click` on any occupied hotbar slot clears that slot.
- Clicking an equipable item shortcut should execute the equip flow.
- Clicking a consumable shortcut should execute the authoritative `use_item` flow.
- Clicking `basic_attack` from `ALT+C` dispatches the current authoritative action that moves into melee range when needed and attacks the current valid target.
- Passive skills belong in the skill book presentation but do not need to occupy hotbar slots by default.
- The HUD should make it obvious which skills are available, cooling down, or not currently usable.
- Online drag-and-drop emits `set_hotbar_state` with the full effective loadout snapshot.
- Clearing a slot persists an empty slot through the same authoritative command.

## Authority Rules

- The client does not decide whether a skill exists, is learned, is still owned by the character, or is currently legal to activate.
- The client does not decide whether an item exists, is owned, is still in a valid container, is consumable, or is legal to use.
- The client does not decide whether an action shortcut such as `basic_attack` can execute against the selected target.
- The client does not invent cooldown duration or ownership; it may re-enable a skill only after the last server-provided cooldown end timestamp has elapsed locally, and the backend still decides whether the retry is legal.
- The client may request hotbar persistence in online mode only through `set_hotbar_state`; it cannot mutate durable bindings directly.
- The backend remains authoritative for learned-skill set, active-passive classification, item ownership, action legality, slot persistence, and activation legality.
- The backend persists accepted slot changes server-side and returns the updated hotbar projection through the same authoritative read model.

## Invariants

- The default hotbar contains exactly `12` slots.
- Each hotbar slot is `32x32px`.
- Skill-book grid slots and character-window navigation buttons are `32x32px`.
- Inventory items dragged to the hotbar stay represented as compact `32x32px` icon shortcuts.
- Additional bars always open upward from the default row.
- The player may open at most `3` hotbars total.
- `ALT+K` opens the skill-book popup.
- `ALT+T`, `ALT+C`, `ALT+N`, and `ALT+U` open the matching character-window panels without duplicating skill icons in the top row.
- The skill book separates `Active` from `Passive`.
- Every skill has an icon.
- Skill details appear on hover, not as always-open text blocks in the grid.
- Hotbar layout and open-bar count persist across disconnect.
- Online drag-and-drop rebinding must round-trip through `set_hotbar_state` before it is durable truth.
- Online slot clearing must round-trip through `set_hotbar_state` before it is durable truth.
- The bar is multitask by definition: never rename the implementation back to skill-only semantics.

## Acceptance Criteria

- The HUD spec clearly supports three stacked horizontal hotbars of `12` compact `32x32px` slots each.
- The hotbar expansion direction is unambiguous.
- The skill-book popup is defined as a vertical rectangular grid panel.
- Active and passive skill categories are explicit and usable.
- Tooltip behavior is defined by hover over icons.
- Drag behavior is visually explicit: the dragged active-skill icon follows the cursor.
- Cooldown feedback is defined as a top-to-bottom clearing overlay on the icon.
- Persistence requirements exist for both slot bindings and visible bar count.
- Skill, item, consumable, and action shortcut semantics are explicit.
- The spec defines both assignment and removal: drag out of bar or `ALT + left click` clears online through `set_hotbar_state`.
- The authoritative persistence path for `skill`, `item`, `action`, and empty slots is explicit.
