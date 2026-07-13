# Interlude Source Study

This dossier maps the gameplay pillars of the studied source tree snapshot used as conceptual reference.

Use it as a high-value gameplay reference for this project, while still translating its concepts into our own architecture, data model, and delivery order.

## Studied Snapshot

- Core gameplay code lives under `java/net/sf/l2j/gameserver`.
- The studied snapshot is on branch `10x`.
- `GameServer.java` boots a classic singleton-heavy server with a broad gameplay and operations footprint loaded into the same runtime.
- The result is a useful design reference, but also a warning about how quickly gameplay, persistence, events, and operations can become entangled.

## Development Order For A New Implementation

1. Character lifecycle, world presence, spawn, despawn, and reconnect.
2. Click-to-move, target acquisition, interaction, and zone gating.
3. Basic combat loop: attack, cast, damage, effects, death, respawn.
4. Skill targeting, AoE, multi-target collection, and formula resolution.
5. NPC and monster behavior, aggro, reward distribution, and loot.
6. Inventory, equipment, paperdoll, item use, shops, and warehouses.
7. Class progression, skill learning, subclass handling, and quest flow.
8. Party, command channel, and cooperative reward sharing.
9. PvP, PK, karma, duel-like rules, and hostile targeting validation.
10. Clan, alliance, reputation, sub-units, and clan skill systems.
11. Territory and competition systems: siege, castle, olympiad, hero.
12. Large world metas and side systems: Seven Signs, manor, festivals, boats.
13. Extended event and live-ops systems after the core game loop is stable.

## Core Source Map

- Server bootstrap and system load order: `java/net/sf/l2j/gameserver/GameServer.java`
- World and visibility: `model/L2World.java`, `model/L2WorldRegion.java`
- Actor hierarchy: `model/actor/Creature.java`, `Playable.java`, `Player.java`, `L2Summon.java`, `L2Npc.java`, `Attackable.java`
- Client intent entry points: `network/clientpackets/Action.java`, `MoveBackwardToLocation.java`, `RequestMagicSkillUse.java`, `UseItem.java`
- Skills and effects: `model/L2Skill.java`, `datatables/SkillTable.java`, `datatables/SkillTreeTable.java`, `skills/l2skills/*`
- Class and stat baselines: `model/base/ClassId.java`, `model/base/PlayerClass.java`, `templates/PlayerTemplate.java`, `tables/CharTemplateTable.java`, `tables/LevelUpTable.java`, `stats/BaseStats.java`, `stats/StatFunctions.java`, `stats/Formulas.java`
- Items and equipment: `datatables/ItemTable.java`, `model/itemcontainer/Inventory.java`, `model/item/instance/ItemInstance.java`
- Social systems: `model/L2Party.java`, `model/L2Clan.java`, `datatables/ClanTable.java`
- Territorial competition: `model/entity/Castle.java`, `model/entity/Siege.java`, `instancemanager/SiegeManager.java`
- Olympiad and hero: `model/olympiad/*`, `model/entity/Hero.java`
- Quest runtime: `scriptings/Quest.java`, `scriptings/QuestState.java`, `scriptings/scripts/quests/*`

## What Matters Most For Us

- The real game backbone is character-centric.
- Most other systems hang off `Player` state, `Creature` combat rules, and `L2World` visibility.
- Targeting and movement are server-authoritative even though the client initiates them.
- Skill behavior is data-driven at the metadata layer and code-driven at the execution layer.
- Progression is split between static content data, persistent character state, and runtime validation rules.
- Late-game systems like siege and olympiad are not side features; they reshape clan, hero, reputation, and item restrictions.

## Reimplementation Guidance

- Preserve the order of dependencies, not the exact class graph.
- Preserve the underlying concepts, state transitions, and responsibility splits, not branch-specific quirks by default.
- End each extraction pass by translating the findings into project documentation or skill guidance for our own build.
- Rebuild the game as bounded services and aggregates instead of global managers calling each other directly.
- Treat client packets as transport details, not as the shape of the domain.
- Keep static content definitions separate from mutable player state.
- Keep rules deterministic and testable without network or database dependencies.
- Add broader event catalogs only after the base MMORPG loop feels solid.

## Read Next

1. [01-character-world-and-lifecycle.md](01-character-world-and-lifecycle.md)
2. [02-combat-stats-skills-and-pve.md](02-combat-stats-skills-and-pve.md)
3. [03-progression-classes-quests-and-items.md](03-progression-classes-quests-and-items.md)
4. [04-social-pvp-and-territorial-systems.md](04-social-pvp-and-territorial-systems.md)
5. [05-implementation-and-scope-guidance.md](05-implementation-and-scope-guidance.md)
6. [06-class-template-and-stat-baseline.md](06-class-template-and-stat-baseline.md)
7. [07-inventory-equipment-and-item-usage.md](07-inventory-equipment-and-item-usage.md)
