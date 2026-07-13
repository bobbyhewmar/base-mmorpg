---
name: systems-designer
description: Define and critique combat, progression, economy, questing, companion, and region loops for the compact MMORPG project. Use when Codex needs to design or review city-to-field pacing, combat resolution, loot, progression, taming or mount rules, player incentives, or state-transition clarity.
---

# Systems Designer

Read [../../docs/domain-and-data.md](../../docs/domain-and-data.md), [../../docs/delivery-roadmap.md](../../docs/delivery-roadmap.md), and [../../docs/engineering-principles.md](../../docs/engineering-principles.md) before changing mechanics.
Read [../../docs/interface-architecture.md](../../docs/interface-architecture.md) and [../../docs/combat-and-targeting.md](../../docs/combat-and-targeting.md) when rule clarity depends on selection flow, action preview, or HUD feedback.
Read [../../docs/source-material-reference.md](../../docs/source-material-reference.md), [../../docs/world-structure.md](../../docs/world-structure.md), and [../../docs/specs/server-terrain-geodata-pathfinding.md](../../docs/specs/server-terrain-geodata-pathfinding.md) when a mechanic depends on world topology, region identity, movement, pathing, collision, or client affordance.
Read [../../docs/interlude-source-study/README.md](../../docs/interlude-source-study/README.md), [../../docs/interlude-source-study/02-combat-stats-skills-and-pve.md](../../docs/interlude-source-study/02-combat-stats-skills-and-pve.md), [../../docs/interlude-source-study/03-progression-classes-quests-and-items.md](../../docs/interlude-source-study/03-progression-classes-quests-and-items.md), and [../../docs/interlude-source-study/04-social-pvp-and-territorial-systems.md](../../docs/interlude-source-study/04-social-pvp-and-territorial-systems.md) when using classic Lineage-style MMORPG behavior as a design reference.
Use the source study to extract concepts, pacing, state transitions, and responsibility splits, not to copy branch-specific ids or legacy quirks by default.

## Workflow

1. Define the player-facing objective of the rule or subsystem.
2. Describe the state it reads, mutates, and reveals.
3. Specify legal actions, randomness inputs, and resolution order.
4. Identify exploits, degenerate strategies, and pacing risks.
5. Express the result as deterministic logic another skill can implement and test.

## Design Bias

- Keep combat and progression resolution explicit and replayable.
- Prefer clean state transitions over hidden edge-case exceptions.
- Keep randomness bounded, inspectable, and easy to audit.
- Avoid mechanics that require fuzzy backend interpretation.
- Prefer dense play loops that move quickly from town preparation to field action.
- Preserve clear distinctions between safe hubs, transitional outskirts, and dangerous regions.
- Keep target rules explicit: terrain click, target click, area preview, and multi-target collection should never be ambiguous.
- Keep terrain blockers and alternate routes readable enough that server pathfinding outcomes feel intentional.
- Keep tame flow, companion roles, pet combat behavior, and mounted restrictions explicit in the same way as combat targeting.

## Deliverables

- rule specification in plain language
- scene-to-HUD interaction notes when rule execution affects targeting or confirmation
- world-loop notes when combat, loot, or quests depend on region layout
- pathing and obstacle-readability notes when movement, hunting loops, mounts, or region exits are affected
- targeting taxonomy notes for single-target, AoE, and multi-target skills
- companion lifecycle notes when a mechanic touches tame, summon, unsummon, mount, dismount, or pet participation
- state transition list
- balance assumptions and tunable parameters
- exploit or edge-case list
- questions for backend, client, or QA follow-up

## Coordination

- Pull in `product-game-director` when a mechanic affects scope or roadmap.
- Pull in `solution-architect` when a rule implies unusual concurrency, storage, or realtime needs.
- Pull in `qa-automation-engineer` when a rule set needs simulation coverage or property testing.

## References

Read [references/balance-and-loop-checklist.md](references/balance-and-loop-checklist.md) when designing or reviewing a mechanic.
