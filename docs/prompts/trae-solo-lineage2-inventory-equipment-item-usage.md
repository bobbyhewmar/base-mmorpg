# Trae SOLO Prompt For Lineage 2 Inventory, Equipment, And Item Usage Extraction

Use this prompt when we want Trae SOLO to inspect the local Lineage 2 source and return structured material about inventory, equipment, item usage, storage, and related item-state rules.

## Goal

Extract the inventory, equipment, and item-usage model from the local Lineage 2 source so we can capture the important concepts and translate them into this project's own documentation, skills, and architecture.

## Prompt To Copy

```text
You are analyzing a local Lineage 2 source tree and must extract information about inventory, item instances, equipment, paperdoll behavior, item usage, and storage-related flows.

OBJECTIVE:
Map how the source defines:
- static item definitions
- runtime item instances
- inventory containers
- equipment and paperdoll rules
- item usage and restrictions
- storage, warehouse, freight, and transfer flows
- stat, skill, and visual recalculation hooks caused by item changes

The output will be consumed by another agent that will update architecture, design documentation, and reusable skill guidance for a new MMORPG project.
Focus on extracting concepts, responsibility boundaries, and runtime flow patterns rather than assuming we will reproduce the source literally.
The result will be transformed into project-specific docs and skills, not treated as a direct porting spec.

IMPORTANT RULES:
- Treat the source as a behavior and systems reference.
- Do not label anything as custom, retail-like, or non-retail. Just describe what the source defines.
- Do not invent flows, restrictions, or persistence rules.
- If a rule is split across packet handlers, container classes, templates, and player checks, explain the split and the call chain.
- If a rule is configured through XML, properties, or data tables, cite both the loader and the data source when possible.
- Always separate:
  - static item definitions
  - runtime item-instance state
  - inventory or container rules
  - item usage policy
  - persistence
  - recalculation hooks
  - UI- or packet-triggered flow
- Always cite the exact file path and the exact class, table, XML, enum, method, or config element that supports each conclusion.
- Prefer extraction over interpretation.
- If something is ambiguous, say it is ambiguous and list the competing source anchors.
- Call out which findings look like durable design concepts versus branch-specific implementation quirks.
- Do not assume packet structure, enum ordinals, schema quirks, or loader hacks must survive into our project unless they support an important gameplay concept.
- When possible, phrase the most useful conclusions so they can be converted into project-specific guidance for systems, backend, data, client, and QA skills.

PRIMARY QUESTIONS TO ANSWER:

1. Which files and classes define static item templates, item categories, body slots, stackability, weight, grade, and equip-ability?
2. Which files and classes define runtime item instances and their mutable state?
3. Which files and classes define inventory containers such as player inventory, warehouse, freight, clan storage, or pet inventory?
4. How does the source implement paperdoll and equip or unequip behavior?
5. How does the source implement item usage validation and restriction policy?
6. Which systems react to item changes by recalculating stats, skills, visual state, penalties, or cooldown-related state?
7. How are stack, split, drop, destroy, trade, warehouse, and storage transitions modeled?
8. Which abstractions, data separations, and rule boundaries matter if we want an inventory model inspired by this source without copying it literally?
9. Which branch-specific quirks or legacy patterns should probably be treated as optional in a new implementation?

SEARCH PRIORITIES:

- Find and inspect at minimum any relevant definitions around:
  - ItemTable
  - ItemInstance
  - Inventory
  - ItemContainer
  - Warehouse
  - PcInventory or equivalent player inventory class if present
  - paperdoll or body-slot enums/constants
  - UseItem
  - DropItem
  - DestroyItem
  - transfer or trade packet handlers
  - warehouse packet handlers
  - multisell, buy, sell, freight, and clan warehouse flows
  - item listeners for stats, skills, augmentation, armor sets, enchant options, or visual changes
  - XML or data files for items and body-slot definitions
  - SQL tables for `items` and any storage-related persistence
  - enums or constants for item types, body parts, crystal grades, stackability, and weapon or armor categories

EXPECTED OUTPUT FORMAT:

Return a single Markdown document with the exact sections below.

# 1. Executive Summary
- 12 to 25 bullets only.
- Focus on the highest-value conclusions about how inventory, equipment, and item usage are actually modeled.

# 2. Source Anchor Map
Create a table with columns:
- Concern
- File Path
- Class/Table/XML
- Why It Matters

# 3. Static Item Definition Model
Explain:
- where item definitions come from
- how they are loaded
- which core item fields exist
- how body slots, item types, weight, stackability, grade, and special flags are represented

# 4. Runtime Item Instance Model
Explain:
- what a runtime item instance stores
- which fields are mutable
- how ownership, location, enchant, augmentation, quantity, and equip state are represented
- how item identity differs from item-template identity

# 5. Inventory And Container Architecture
Explain:
- player inventory structure
- paperdoll structure
- warehouse or freight structure
- clan or pet storage if present
- how container-specific rules differ

# 6. Equip, Unequip, And Paperdoll Rules
Explain:
- legal equip-slot rules
- dual-slot or conflicting-slot rules
- equip and unequip flow
- how the source applies paperdoll changes
- how visual state, skills, or stat listeners react

# 7. Item Usage And Restriction Policy
Explain:
- how item-use intent enters the runtime
- where validation happens
- which states can block usage
- zone, event, Olympiad, class, pet, fishing, trade, store, death, karma, or transformation restrictions if present
- how consumables, scrolls, quest items, teleport items, and equipable items differ in policy

# 8. Inventory State Transitions
Describe the major flows for:
- add item
- remove item
- stack or split item
- equip item
- unequip item
- drop item
- destroy item
- move to warehouse or freight
- move back from storage
- trade or multisell related transitions

Prefer step-by-step flow descriptions with source anchors.

# 9. Recalculation Hooks And Side Effects
Explain which systems react to inventory or equipment changes, including:
- stat recalculation
- skill grants or removals
- augmentation effects
- armor-set behavior
- expertise or weight penalties
- visible gear or appearance updates
- shortcut or cooldown interactions if relevant

# 10. Persistence Model
Explain:
- which tables hold item-instance truth
- how container location is persisted
- how equip state is persisted
- how warehouse or freight state is persisted
- which fields look authoritative versus cached or convenience-oriented

# 11. Concept Extraction And Implementation Guidance
Provide a flat checklist of what a new implementation should preserve conceptually if we want:
- Lineage 2-like item-template separation
- Lineage 2-like inventory and paperdoll behavior
- Lineage 2-like item-use restriction policy
- Lineage 2-like storage and transfer rules

Phrase this section so another agent can turn it into project documentation and skill guidance with minimal rewriting.

# 12. Branch-Specific Quirks To Treat Carefully
List patterns that appear real in this branch but should not automatically become requirements for our project.

# 13. Important Data Structures And Enums
List the key classes, enums, tables, XML groups, and config switches that shape this entire area.

# 14. Open Questions And Ambiguities
List only unresolved items that would need a second pass.

OUTPUT CONSTRAINTS:
- Use exact file paths relative to the configured source tree.
- When citing code, cite short anchors like class name, method name, enum name, XML node, loader name, or packet handler.
- Do not dump huge code blocks.
- Do not flatten restrictions into vague summaries.
- Do not skip persistence or recalculation hooks.
- If the material is too large for one pass, split the output into clearly labeled parts but keep the same section structure.
```

## What To Send Back Here

After Trae SOLO finishes, paste the result in parts if needed, preserving section titles and source anchors.

If the output is large, send it in this order:

1. `1. Executive Summary`
2. `2. Source Anchor Map`
3. `3. Static Item Definition Model`
4. `4. Runtime Item Instance Model`
5. `5. Inventory And Container Architecture`
6. `6. Equip, Unequip, And Paperdoll Rules`
7. `7. Item Usage And Restriction Policy`
8. `8. Inventory State Transitions`
9. `9. Recalculation Hooks And Side Effects`
10. `10. Persistence Model`
11. `11. Concept Extraction And Implementation Guidance`
12. `12. Branch-Specific Quirks To Treat Carefully`
13. `13. Important Data Structures And Enums`
14. `14. Open Questions And Ambiguities`

## Why This Shape Works

- It separates static item data from runtime item-instance state.
- It captures both storage rules and item-use policy, which are often mixed together in legacy MMO code.
- It gives us enough structure to turn the extraction into project-specific docs and skill guidance.
- It helps us preserve the model while still redesigning the implementation.
