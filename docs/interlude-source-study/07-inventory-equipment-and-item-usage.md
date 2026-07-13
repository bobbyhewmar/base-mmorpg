# Inventory, Equipment, And Item Usage

This document consolidates the extracted inventory pass from the studied source so we can understand its item and equipment model, then re-express the useful concepts in our own implementation.

## Highest-Value Findings

- Static item truth lives in XML under `dist/gameserver/data/items/` and is loaded into immutable `ItemTemplate` objects.
- Runtime item truth lives in `ItemInstance`, which separates template identity from instance identity and holds mutable ownership, quantity, location, enchant, attributes, augmentation, timers, and visual overrides.
- Inventory behavior is layered: `ItemContainer` owns shared mutation semantics, `Inventory` adds paperdoll and equip logic, and specialized containers like player inventory, warehouse, freight, and pet inventory build on top.
- Equip state is effectively container membership plus slot, not a separate equipment table.
- Paperdoll is a fixed slot model with runtime conflict resolution layered on top of static body-slot metadata.
- Item-use policy is intentionally layered across packet/runtime state checks, template conditions, instance policy helpers, and item handlers.
- Trade and multisell do not mutate inventory immediately on intent; they stage validation first and only mutate after capacity, weight, and rule checks pass.
- Partial stack removal creates a new `ItemInstance` fragment with a new object identity instead of pretending one stack exists in two places.
- Inventory side effects are listener-driven: stats, skills, augmentation, armor sets, appearance, offhand ammo or bait behavior, and penalties react to equip or unequip callbacks.
- Weight and expertise penalties are derived from current equipment and current inventory state rather than stored directly on items.
- Warehouse, freight, clan storage, and pet inventory reuse the same underlying item-instance model but change owner semantics, access rules, and admissibility policy.
- Several details look durable as concepts, while others look like branch-specific implementation baggage that we should not inherit blindly.

## Source Anchors

- `java/l2/gameserver/data/xml/parser/ItemParser.java`
- `java/l2/gameserver/data/xml/holder/ItemHolder.java`
- `java/l2/gameserver/templates/item/ItemTemplate.java`
- `java/l2/gameserver/templates/item/Bodypart.java`
- `java/l2/gameserver/templates/item/ItemFlags.java`
- `java/l2/gameserver/templates/item/WeaponTemplate.java`
- `java/l2/gameserver/templates/item/ArmorTemplate.java`
- `java/l2/gameserver/templates/item/EtcItemTemplate.java`
- `java/l2/gameserver/model/items/ItemInstance.java`
- `java/l2/gameserver/model/items/ItemContainer.java`
- `java/l2/gameserver/model/items/Inventory.java`
- `java/l2/gameserver/model/items/PcInventory.java`
- `java/l2/gameserver/model/items/PetInventory.java`
- `java/l2/gameserver/model/items/Warehouse.java`
- `java/l2/gameserver/model/items/PcWarehouse.java`
- `java/l2/gameserver/model/items/ClanWarehouse.java`
- `java/l2/gameserver/model/items/PcFreight.java`
- `java/l2/gameserver/network/l2/c2s/UseItem.java`
- `scripts/handler/items/EquipableItem.java`
- `java/l2/gameserver/utils/ItemFunctions.java`
- `java/l2/gameserver/network/l2/c2s/RequestDropItem.java`
- `java/l2/gameserver/network/l2/c2s/RequestDestroyItem.java`
- `java/l2/gameserver/network/l2/c2s/AddTradeItem.java`
- `java/l2/gameserver/network/l2/c2s/TradeDone.java`
- `java/l2/gameserver/network/l2/c2s/SendWareHouseDepositList.java`
- `java/l2/gameserver/network/l2/c2s/SendWareHouseWithDrawList.java`
- `java/l2/gameserver/network/l2/c2s/RequestPackageSend.java`
- `java/l2/gameserver/network/l2/c2s/RequestBuyItem.java`
- `java/l2/gameserver/network/l2/c2s/RequestSellItem.java`
- `java/l2/gameserver/network/l2/c2s/RequestMultiSellChoose.java`
- `java/l2/gameserver/model/items/listeners/*.java`
- `java/l2/gameserver/dao/ItemsDAO.java`
- `dist/gameserver/sql/install/items.sql`

## Durable Model Concepts

### Separate template from instance

- `ItemTemplate` defines what the item is.
- `ItemInstance` defines who owns it, where it is, how many exist in that stack, and which mutable upgrades, overlays, or timers it currently carries.
- Static conditions, flags, attached skills, stat funcs, and option pools belong on the template side.
- Quantity, enchant, augmentation, attributes, visibility override, and location belong on the instance side.

This split should survive even if our own file formats, ids, or schema layout differ.

### Separate generic container rules from equip-capable rules

- `ItemContainer` owns shared mutation behavior such as add, remove, destroy, split, merge, and persistence callbacks.
- `Inventory` adds paperdoll semantics, weight or capacity checks, equip listeners, and equip-slot transitions.
- Warehouse-like containers are storage-oriented rather than equip-capable.

This suggests our new project should not let every storage type reinvent mutation logic.

### Treat equip state as placement plus slot

- The branch persists equip state through location plus slot rather than through a separate equipment table.
- The durable concept is not the exact schema shape.
- The durable concept is that item placement and equip-slot occupancy should form one coherent model, not two drifting truths.

### Keep item-use policy layered

The extracted branch distributes item-use rules across:

- common runtime state checks
- data-driven template conditions
- instance policy helpers such as drop or store or trade legality
- item-category-specific handlers

This is a strong concept to keep because it avoids shoving every rule into one packet handler or one god-service.

### Keep side effects event-driven

Equip and unequip side effects should be reactions to inventory state changes, not duplicated logic in every caller.

The extracted branch uses listeners for:

- stat recalculation
- item-granted skills
- augmentations
- enchant options
- armor sets
- ammo or bait behavior
- appearance updates

We should preserve the separation, even if we use domain events or explicit hooks instead of the same listener classes.

## Item Definition Model

Static item definitions are XML-backed in the studied branch.

Confirmed concept groups include:

- item category
- body slot or body-slot mask
- weight
- stackability
- grade or rarity-like progression marker
- reuse or cooldown metadata
- crystallization or material metadata
- destroy, drop, trade, store, enchant, augment, freight, or attribute-related flags
- attached stat funcs
- attached skills and triggers
- template-side conditions
- base elemental values
- enchant option pools

The most important takeaway for our project is not the XML format itself.
The important takeaway is to keep static item definitions data-driven and attach rule metadata to the template layer rather than scattering it through code.

## Runtime Item Model

The extracted `ItemInstance` model shows a useful separation of concerns:

- one stable instance id per concrete item stack or unique item
- one template id that points at immutable item definition data
- mutable ownership and placement
- mutable quantity
- mutable enhancement or override state
- mutable lifecycle timers or temporary status

The branch also shows two useful behavioral patterns:

- non-stackable items clamp to one per instance
- partial transfer of a stack produces a new runtime instance fragment rather than vague partial ownership

We should preserve those semantics conceptually, even if our own persistence model is cleaner.

## Inventory And Storage Model

The studied branch uses one item-centric persistence idea across multiple container types.

Durable concepts:

- player inventory
- equip-capable paperdoll inventory
- pet inventory
- private storage
- clan storage
- cross-character freight or delivery-like storage
- optional service-specific containers such as refund windows or delayed delivery

The branch-specific part is how many of those special cases it keeps as first-class storage locations.
For our own project, the important part is to keep owner semantics, access policy, capacity rules, and admissibility rules explicit for each storage domain.

## Equip And Paperdoll Model

The extracted equip model reinforces several important design ideas:

- body-part legality is static metadata
- slot-conflict resolution is runtime policy
- equip or unequip should flow through one central swap path
- displaced gear should be handled automatically and consistently
- side effects should attach to the swap path rather than to packet handlers

Observed conflict-resolution patterns include:

- dual-hand and left-hand dependencies
- arrow and bait compatibility
- full armor versus legs
- formal wear conflicts
- hair and dual-hair conflicts
- ear and finger auto-placement

We do not need to copy those exact slot families unless our own game wants them.
But we do need one coherent equip-conflict subsystem instead of ad hoc checks spread everywhere.

## Item Use Policy Model

The studied branch does not have one pure item-policy service.
Instead it layers:

1. generic user-state gates
2. template conditions
3. instance-specific transition legality
4. handler-defined category behavior

Useful blocking-state concepts found in the branch include:

- store or workshop state
- active trade or request state
- fishing state
- dead or out-of-control state
- mount restrictions for weapon-hand equip
- pet-state restrictions
- karma or PvP-state restrictions for some service loops

The new project should keep the layered policy idea, but centralize the resulting domain logic more cleanly than packet-era branching.

## Inventory State Transitions

The extracted branch confirms a strong transition model:

- add item
- remove item
- split stack
- merge stack
- equip
- unequip
- drop
- destroy
- move to storage
- withdraw from storage
- stage for trade
- commit trade
- validate and execute multisell-like exchanges

Two especially strong concepts to preserve are:

- staged validation before mutation for trade and exchange systems
- item-centric transitions with explicit split or merge behavior

## Persistence Concepts

The extracted branch stores item-instance truth in one core table plus extension tables.

The durable concept for our project is:

- keep item-instance identity and placement authoritative
- keep mutable enhancement or attribute data attached to the instance
- keep equip placement reconstructible from authoritative state
- keep container membership explicit
- keep optional extension state separate when it meaningfully reduces coupling

We should not feel obligated to keep stored procedures, exact table names, or the same extension breakdown.

## Branch-Specific Quirks To Treat Carefully

The extraction surfaced details that look real in the branch but should not become automatic requirements for our own game:

- negative pet bodypart placeholders
- legacy `TYPE1_*` and `TYPE2_*` integers
- freight policy that does not appear to consult all template flags consistently
- hardcoded warehouse and freight fees in packet handlers
- refund modeled as `VOID` items
- appearance overlays persisted as instance-side visible item ids
- carried runes treated like pseudo-equipped listeners
- quest items excluded from effective capacity calculations

These are useful to know about, but they should only be adopted deliberately if they support our design.

## What This Means For Our Project

- We should model immutable item templates separately from mutable item instances.
- We should model container membership as first-class state.
- We should keep one shared mutation model for inventory and storage transitions.
- We should keep equip-capable inventory logic separate from storage-only container logic.
- We should keep equip state coherent with placement and slot occupancy.
- We should keep staged validation before trade or exchange mutations.
- We should keep inventory-driven side effects behind explicit hooks or events.
- We should keep weight, expertise, and similar penalties derived from current state rather than persisted into item rows.
- We should turn branch findings into our own domain services, aggregates, and persistence model instead of copying container subclasses row-for-row.
