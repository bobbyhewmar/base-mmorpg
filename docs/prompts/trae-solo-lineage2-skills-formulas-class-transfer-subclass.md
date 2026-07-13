# Trae SOLO Prompt For Lineage 2 Skills, Formulas, Class Transfer, And Subclass Extraction

Use this prompt when we want Trae SOLO to inspect the local Lineage 2 source and return structured material about skill logic, combat formulas, class transfer flow, and subclass behavior.

## Goal

Extract the skill system, stat-formula model, and class progression flow from the local Lineage 2 source so we can capture the important concepts and update this project's documentation and skills with implementation guidance for our own architecture.

## Prompt To Copy

```text
You are analyzing a local Lineage 2 source tree and must extract canonical information about skills, formulas, class transfer, and subclass behavior.

OBJECTIVE:
Map how the source defines:
- skill metadata and runtime behavior
- skill learning and progression trees
- combat and stat formulas
- class transfer flow
- subclass rules and lifecycle

The output will be consumed by another agent that will update architecture, design documentation, and reusable skill guidance for a new MMORPG project.
Focus on extracting concepts, responsibility boundaries, and runtime flow patterns rather than assuming we will reproduce the source literally.
The result will be transformed into project-specific docs and skills, not treated as a direct porting spec.

IMPORTANT RULES:
- Treat the source as canonical.
- Do not label anything as custom, retail-like, or non-retail. Just describe what the source defines.
- Do not invent values, formulas, or flow steps.
- If a formula is distributed across multiple classes, explain the split and the call chain.
- If a rule is configured through XML, properties, or data tables, cite both the loader and the data source when possible.
- Always separate:
  - static definitions
  - runtime resolution logic
  - persistence
  - recalculation hooks
  - UI- or packet-triggered flow
- Always cite the exact file path and the exact class, table, XML, enum, method, or config element that supports each conclusion.
- Prefer extraction over interpretation.
- If something is ambiguous, say it is ambiguous and list the competing source anchors.
- Call out which findings look like durable design concepts versus branch-specific implementation quirks.
- Do not assume packet structure, enum ordinals, or schema quirks must survive into our project unless they support an important gameplay concept.
- When possible, phrase the most useful conclusions so they can be converted into project-specific guidance for systems, backend, data, client, and QA skills.

PRIMARY QUESTIONS TO ANSWER:

1. Which files and classes define skill identity, metadata, target type, cast rules, reuse, and effect behavior?
2. Which files and classes resolve combat formulas such as damage, hit chance, crit chance, speed, attack speed, cast speed, resist, success rate, and abnormal effects?
3. Which parts of the skill system are static data and which parts are runtime logic?
4. How are skills learned, restored, rewarded, enchanted, or recalculated?
5. How does the source implement class transfer, class master interactions, and class-based progression gates?
6. How does subclass switching work in practice, including restrictions, persistence, stat restoration, and skill restoration?
7. Which systems recalculate effective power after class change, subclass switch, equip change, passive skill changes, buffs, or login restoration?
8. Which source assets, abstractions, or responsibility splits matter if we want behavior inspired by this source without copying it literally?
9. Which branch-specific quirks or legacy patterns should probably be treated as optional in a new implementation?

SEARCH PRIORITIES:

- Find and inspect at minimum any relevant definitions around:
  - L2Skill
  - SkillTable
  - SkillTreeTable
  - effect classes
  - formula or stat calculation classes
  - Creature and Player stat access paths
  - class master NPC logic
  - subclass classes and handlers
  - skill save and restore flows
  - client packet handlers that trigger skill use or class progression actions
  - XML or data files for skill definitions, skill trees, and class progression data
  - enums or constants for skill target types, effect types, stat types, abnormal types, operate types, and class IDs

EXPECTED OUTPUT FORMAT:

Return a single Markdown document with the exact sections below.

# 1. Executive Summary
- 12 to 25 bullets only.
- Focus on the highest-value conclusions about how skills, formulas, class transfer, and subclass behavior are actually modeled.

# 2. Source Anchor Map
Create a table with columns:
- Concern
- File Path
- Class/Table/XML
- Why It Matters

# 3. Skill System Architecture
Explain:
- where skill definitions come from
- how they are loaded
- how runtime lookup works
- how skill metadata is structured
- how skill behavior is specialized
- how effect templates or effect handlers plug into skill execution

# 4. Skill Metadata And Runtime Semantics
Extract the most important confirmed dimensions of a skill definition, such as:
- skill id
- level
- name
- operate type
- target type
- cast range
- effect range
- hit time / cast time
- reuse delay
- mp or hp cost
- offensive vs supportive flags
- self effect support
- abnormal type or effect grouping
- condition or weapon restrictions
- whether it is toggle, aura, single-target, self-centered, target-centered, chain-like, or other relevant category

Explain where each of those concepts lives in code or data.

# 5. Formula And Derived Stat Model
Separate static inputs from computed outputs.
For each major formula family, explain:
- where the formula lives
- which inputs it uses
- which actor stats or modifiers affect it
- when it is evaluated

Cover at least:
- physical damage
- magical damage
- hit / miss
- crit chance and crit damage
- attack speed
- cast speed
- movement speed if formula-driven
- accuracy / evasion
- success chance for debuffs or abnormal effects
- defense / mitigation / shield-like interactions if present
- regen or restoration if formula-driven

# 6. Skill Learning, Restore, And Progression Hooks
Explain:
- how class skill trees are loaded
- how learnable skills are gated
- how character skills are saved and restored
- where reward skills or automatic skill grants happen
- what is recalculated on login, level up, class transfer, subclass switch, enchant, or passive changes

# 7. Class Transfer Flow
Explain the flow for class transfer or class advancement:
- which NPCs or services trigger it
- which files define the rules
- prerequisites
- class lineage constraints
- item or quest requirements if present
- state updates applied on success
- skill or status recalculation after the transfer

# 8. Subclass System
Explain the subclass model:
- how subclass data is stored
- limits and eligibility
- blocked states and restrictions
- subclass creation flow
- subclass switch flow
- what is restored, recalculated, or cleared
- how skills and shortcuts interact with subclass changes
- how summons, weight, olympiad, events, or similar systems restrict subclass actions if the source defines that

# 9. Runtime Event Flow
Describe the important runtime flows for:
- using a skill
- validating a target
- applying effects
- recalculating stats
- performing class transfer
- switching subclass

Prefer step-by-step flow descriptions with source anchors.

# 10. Concept Extraction And Implementation Guidance
Provide a flat checklist of what a new implementation should preserve conceptually if we want:
- Lineage 2-like skill behavior
- Lineage 2-like formula behavior
- Lineage 2-like class transfer rules
- Lineage 2-like subclass lifecycle
Phrase this section so another agent can turn it into project documentation and skill guidance with minimal rewriting.

# 11. Branch-Specific Quirks To Treat Carefully
List patterns that appear real in this branch but should not automatically become requirements for our project.

# 12. Important Data Structures And Enums
List the key classes, enums, tables, XML groups, and config switches that shape this entire area.

# 13. Open Questions And Ambiguities
List only unresolved items that would need a second pass.

OUTPUT CONSTRAINTS:
- Use exact file paths relative to the configured source tree.
- When citing code, cite short anchors like class name, method name, enum name, XML node, loader name, or packet handler.
- Do not dump huge code blocks.
- Do not flatten formulas into vague summaries.
- Do not skip subclass restrictions or recalculation hooks.
- If the material is too large for one pass, split the output into clearly labeled parts but keep the same section structure.
```

## What To Send Back Here

After Trae SOLO finishes, paste the result in parts if needed, preserving section titles and source anchors.

If the output is large, send it in this order:

1. `1. Executive Summary`
2. `2. Source Anchor Map`
3. `3. Skill System Architecture`
4. `4. Skill Metadata And Runtime Semantics`
5. `5. Formula And Derived Stat Model`
6. `6. Skill Learning, Restore, And Progression Hooks`
7. `7. Class Transfer Flow`
8. `8. Subclass System`
9. `9. Runtime Event Flow`
10. `10. Concept Extraction And Implementation Guidance`
11. `11. Branch-Specific Quirks To Treat Carefully`
12. `12. Important Data Structures And Enums`
13. `13. Open Questions And Ambiguities`

## Why This Shape Works

- It separates skill definitions from formulas, which is essential for a strong adaptation.
- It captures both player-facing rules and backend recalculation paths.
- It gives us exact anchors for class transfer and subclass logic instead of leaving that behavior as folklore.
- It produces output that can be folded directly into the project's source-study documentation and implementation roadmap without locking us into a code transplant.
