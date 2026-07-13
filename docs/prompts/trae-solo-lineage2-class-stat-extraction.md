# Trae SOLO Prompt For Lineage 2 Class And Stat Extraction

Use this prompt when we want Trae SOLO to inspect the local Lineage 2 source and return structured material that can be folded back into this project's documentation.

## Goal

Extract the class, race, template-stat, and progression-baseline model from the local Lineage 2 source so we can capture the important concepts and translate them into this project's own documentation, skills, and architecture.

## Prompt To Copy

```text
You are analyzing a local Lineage 2 source tree and must extract canonical information about playable classes and their status templates.

OBJECTIVE:
Map how the source defines class identity and the baseline status of each playable class so another agent can update architecture, design documentation, and reusable skill guidance without having to re-read the codebase.
Focus on extracting concepts, separations of responsibility, and implementation-relevant patterns rather than arguing that our project must copy the branch literally.
The result will be transformed into project-specific docs and skills, not treated as a direct porting spec.

IMPORTANT RULES:
- Treat the source as canonical.
- Do not label anything as custom, retail-like, or non-retail. Just describe what the source defines.
- Do not invent values.
- If a value is derived rather than stored directly, explain exactly where the derivation happens.
- Separate static template values from runtime formulas and from persistent character state.
- Always cite the exact file path and the exact class, table, XML, enum, method, or config element that supports each conclusion.
- Prefer extraction over interpretation.
- When something is ambiguous, say it is ambiguous and list the competing source anchors.
- Call out which findings look like durable design concepts versus branch-specific implementation quirks.
- Do not recommend copying branch-specific ids, loader hacks, or legacy columns unless they are essential to the underlying model.
- When possible, phrase the most useful conclusions so they can be converted into project-specific guidance for systems, backend, data, and QA skills.

PRIMARY QUESTIONS TO ANSWER:

1. Which files and classes define playable race, class lineage, and class identifiers?
2. Which files define base template stats for player characters and classes?
3. Which status fields exist as baseline template values?
4. Which combat or derived stats come from formulas instead of static templates?
5. Which values differ by race, by base class, by class tier, or by subclass handling?
6. Which runtime systems recalculate or restore stats on login, class change, subclass switch, item equip, buff apply, or skill learn?
7. Which tables, XML files, enums, loaders, or conceptual separations matter if we want to preserve the model without copying the implementation literally?
8. Which parts belong to:
   - class identity
   - static class template
   - current status pools
   - derived combat stats
   - progression and skill tree gates
9. Which parts appear to be branch-specific quirks or legacy baggage that we should probably treat as optional rather than canonical in our own build?

SEARCH PRIORITIES:

- Find and inspect at minimum any relevant definitions around:
  - ClassId
  - PlayerTemplate
  - CharTemplateTable
  - LevelUpTable
  - PlayerClass
  - BaseStats
  - Player
  - Creature
  - formulas or stats calculation classes
  - skill tree loaders
  - subclass handling
  - XML or data files that load player templates
  - SQL or data files that load player templates
  - char_templates.sql
  - lvlupgain.sql
  - attribute_bonus.xml
  - enums or constants for STR, DEX, CON, INT, WIT, MEN, HP, MP, CP, speed, crit, atk speed, cast speed, load, collision, and other baseline actor values

EXPECTED OUTPUT FORMAT:

Return a single Markdown document with the exact sections below.

# 1. Executive Summary
- 10 to 20 bullets only.
- Focus on the highest-value conclusions about how class status is actually modeled.

# 2. Source Anchor Map
Create a table with columns:
- Concern
- File Path
- Class/Table/XML
- Why It Matters

# 3. Playable Race And Class Lineage
Explain:
- playable races
- base class families
- class transfer lineage
- class IDs and hierarchy rules
- subclass-related constraints that affect class identity

# 4. Static Template Stat Model
Explain exactly which baseline fields are stored in templates.
Include a normalized list of all baseline fields you found, such as:
- STR
- DEX
- CON
- INT
- WIT
- MEN
- base HP
- base MP
- base CP
- HP/MP/CP growth or modifiers if present
- run speed
- walk speed
- attack speed
- cast speed
- crit rate
- accuracy
- evasion
- load or carry limit
- collision height/radius
- spawn data or other class-linked template fields if relevant

# 5. Derived Stat And Formula Model
Separate what is static from what is computed.
For each important derived stat, explain:
- where the formula lives
- which base inputs it uses
- whether class template, race, level, equipment, buff, passive, or status state can alter it

# 6. Current Status Pools Versus Permanent Stats
Explain the difference between:
- max stats
- current HP/MP/CP
- derived combat values
- restored or recalculated values on login or class transitions

# 7. Progression Hooks That Change Stats
Explain where stats or effective power are altered by:
- level up
- class transfer
- subclass switch
- skill learn or restore
- equip/unequip
- passive skills
- buffs/debuffs
- pets, summons, or mount state if they influence player status

# 8. Concept Extraction And Implementation Guidance
Provide a flat checklist of the concepts and separations a new implementation should preserve if it wants to capture the same class-stat model without copying the old codebase literally.
Phrase this section so another agent can turn it into project documentation and skill guidance with minimal rewriting.

# 9. Branch-Specific Quirks To Treat Carefully
List implementation details that seem real in this branch but should not automatically become requirements for our new project.

# 10. Per-Class Extraction Tables
Produce one table per playable class or, if the source is organized differently, one table per template group.
Each row should include as many confirmed fields as possible:
- class_id
- class_name
- race
- parent_class
- transfer_tier or class_level
- template_source
- base_STR
- base_DEX
- base_CON
- base_INT
- base_WIT
- base_MEN
- base_HP_related_fields
- base_MP_related_fields
- base_CP_related_fields
- move_speed_fields
- attack_speed_fields
- cast_speed_fields
- crit_related_fields
- collision_fields
- notes

If some of these values are not stored per class, say exactly where they come from instead of leaving the reason implicit.

# 11. Open Questions And Ambiguities
List only unresolved items that would need a second pass.

OUTPUT CONSTRAINTS:
- Use exact file paths relative to the configured source tree.
- When citing code, cite short anchors like class name, method name, enum name, XML node, or loader name.
- Do not dump huge code blocks.
- Do not summarize away the per-class data.
- If the source has too many classes for one pass, split the output into clearly labeled parts but keep the same section structure.
```

## What To Send Back Here

After Trae SOLO finishes, paste the result in parts if needed, preserving section titles and source anchors.

If the output is large, send it in this order:

1. `1. Executive Summary`
2. `2. Source Anchor Map`
3. `3. Playable Race And Class Lineage`
4. `4. Static Template Stat Model`
5. `5. Derived Stat And Formula Model`
6. `6. Current Status Pools Versus Permanent Stats`
7. `7. Progression Hooks That Change Stats`
8. `8. Concept Extraction And Implementation Guidance`
9. `9. Branch-Specific Quirks To Treat Carefully`
10. `10. Per-Class Extraction Tables`
11. `11. Open Questions And Ambiguities`

## Why This Shape Works

- It gives us both strategic conclusions and raw source anchors.
- It separates template data from formulas, which is critical for implementing Lineage-like stats correctly.
- It helps us preserve the model while still redesigning the implementation.
- It produces per-class tables in a form that can be folded into our documentation and later into data definitions.
