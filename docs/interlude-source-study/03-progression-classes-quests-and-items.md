# Progression, Classes, Quests, And Items

## Character Progression Is Split Across Three Layers

The studied source spreads progression across:

- static content data
- persistent character state
- runtime gatekeeping logic

That pattern is still useful, but the implementation should be cleaner in a new project.

## Classes And Templates

Class identity depends on:

- `model/base/ClassId`
- `model/base/PlayerClass`
- `templates/PlayerTemplate`
- `tables/CharTemplateTable`
- `tables/LevelUpTable`
- `dist/gameserver/sql/install/class_list.sql`
- `dist/gameserver/sql/install/char_templates.sql`
- `dist/gameserver/sql/install/lvlupgain.sql`

These split class behavior into different responsibilities:

- `ClassId` is the canonical runtime class tree, numeric id contract, parent chain, race, mage flag, and displayed profession tier.
- `PlayerClass` is a policy enum used heavily by subclass eligibility and exclusion logic.
- `PlayerTemplate` is the runtime player-template object backed by `CharTemplate`.
- `CharTemplateTable` joins `class_list` and `char_templates`, then materializes `PlayerTemplate` instances.
- `LevelUpTable` supplies class-and-level HP, MP, and CP curves separately from the static template row.

Important corrections from the extracted source pass:

- `PcTemplate` does not exist in this source tree.
- the authoritative static player-template source is SQL, not XML
- playable races in this branch are `Human`, `Elf`, `Dark Elf`, `Orc`, and `Dwarf`
- gender is not loaded from separate class rows; the loader synthesizes male and female runtime templates from one SQL row and stores female templates under `classId | 256`
- fourth classes often reuse the same static STR, DEX, CON, INT, WIT, and MEN family as earlier classes while still having distinct HP, MP, and CP curves through `lvlupgain`

Detailed class-template and status-baseline notes now live in [06-class-template-and-stat-baseline.md](06-class-template-and-stat-baseline.md).

Class progression then connects to:

- skill trees
- subclass eligibility
- noblesse and hero systems
- item restrictions
- village master and class master interactions

## Skill Learning

Skill learning is split across:

- `datatables/SkillTable.java` for runtime skill lookup
- `datatables/SkillTreeTable.java` for learnable progression trees
- player skill restore and save inside `Player.java`

`SkillTreeTable` loads:

- class skill trees
- fishing skills
- dwarven craft expansions
- pledge skills
- enchant skill trees and enchant data

This matters because Lineage-style progression is not only level based. It is also class based, clan based, and sometimes content-path based.

The extracted source pass also confirmed an important inheritance rule:

- class skill trees are XML-backed
- child classes receive parent class skills at load time through `SkillAcquireHolder.addAllNormalSkillLearns()`

## Reward Skills And Character Maturation

`Player.rewardSkills()` appears as a recurring progression hook.

That means the old source frequently re-evaluates:

- expertise or lucky-like rewards
- noblesse or hero additions
- restoration after login or class changes

For a rewrite, progression recalculation should be explicit and event-driven:

- on level up
- on class change
- on subclass switch
- on noblesse or hero gain
- on item or passive changes that alter allowed skills

## Subclass

Subclass handling is one of the densest progression systems in the source.

Main references:

- `model/base/SubClass.java`
- `model/actor/instance/L2ClassMasterInstance.java`
- `model/base/PlayerClass.java`
- related village master and trainer flows

Observed rules include:

- maximum subclass count is configurable
- subclass actions are blocked during cast, summon presence, overweight, olympiad registration, and tournament participation
- subclass data tracks `class_id`, `class_index`, `exp`, `sp`, and `level`
- subclass persistence also tracks `curHp`, `curMp`, and `curCp`
- this source makes starting subclass level configurable through server configuration
- legality is stricter than ancestry alone and combines exclusion families, race or teacher restrictions, config flags, already-owned classes, and parent or child conflict checks
- `PlayerClass.values()[classId]` is used directly in subclass logic, so enum ordering is part of the class-id contract

Important conclusion:

- subclass is not just a cosmetic alternate build
- it is a parallel progression track with its own skill and state restore path
- it also owns saved current HP, MP, and CP per subclass

Another important extracted detail:

- `character_subclasses.maxHp`, `maxMp`, and `maxCp` are written but are not the runtime source of truth during restore
- effective max HP, MP, and CP come from runtime recalculation against class, level, base stats, and modifiers

## Noblesse And Hero

Noblesse and hero are separate but related progression layers.

The source shows:

- noblesse skill grants through `SkillTable`
- class master flows that can grant noblesse
- raid-based noblesse rewards in some modules
- olympiad-driven hero selection
- hero diaries, fights, messages, and item cleanup via `model/entity/Hero.java`

This tells us high-end progression is deeply connected to competition and clan prestige, not only PvE leveling.

## Quest Runtime

The quest system revolves around:

- `scriptings/Quest.java`
- `scriptings/QuestState.java`
- quest scripts under `scriptings/scripts/quests`

`QuestState` stores:

- quest state
- per-quest variables
- registered quest items
- quest completion and repeatable cleanup behavior

Persistence uses `character_quests`.

Important behaviors:

- quests can subscribe to death notifications
- quest conditions are stored as variables such as `cond`
- quest scripts react to events like kill, talk, and attack
- many retail quest scripts already contain party-aware helper patterns

For our project, this strongly suggests:

- keep quest runtime event-driven
- persist quest variables separately from the core character row
- let quests observe domain events instead of embedding quest logic into player code

## Inventory And Equipment

The item and equipment stack is split across:

- `data/xml/parser/ItemParser.java`
- `data/xml/holder/ItemHolder.java`
- `templates/item/ItemTemplate.java`
- `model/items/ItemInstance.java`
- `model/items/ItemContainer.java`
- `model/items/Inventory.java`
- `model/items/PcInventory.java`
- `model/items/Warehouse.java`

Important corrections from the extracted inventory pass:

- static item truth is XML-backed under `dist/gameserver/data/items`
- immutable item-template definitions and mutable item-instance state are distinct
- generic container mutation behavior is separate from equip-capable paperdoll behavior
- equip state is effectively placement plus slot, not a separate equipment table
- storage variants such as warehouse, freight, clan storage, and pet inventory reuse the same item-instance model while changing owner semantics and access policy

`Inventory` and related container classes manage:

- the paperdoll
- equip and unequip rules
- listener notifications on gear changes
- dual-slot and body-slot behavior
- weight and capacity validation
- storage and transfer semantics depending on container type
- visual overrides and restore behavior

This is not just storage. It is a rules engine for:

- legal equip slots
- stat recalculation triggers
- loadout identity
- visible character appearance changes driven by equipped gear
- stack split and merge semantics
- staged trade and exchange mutation

Detailed inventory, paperdoll, and item-usage notes now live in [07-inventory-equipment-and-item-usage.md](07-inventory-equipment-and-item-usage.md).

## Item Use

`UseItem.java` shows how large item policy becomes in a long-running MMO.

It checks:

- trade state
- store mode
- quest item bans
- death or crowd-control state
- zone-specific item restrictions
- olympiad restrictions
- class-specific item restrictions
- tournament and event restrictions
- fishing state
- pet item rules
- teleport restrictions for karma players

The extracted inventory pass sharpened the deeper model:

- item use is intentionally layered across generic runtime gates, template conditions, instance transition helpers, and item-category handlers
- item category enum alone does not define behavior; handlers still own category-specific runtime logic
- item transitions such as trade, multisell, drop, destroy, warehouse deposit, and freight are validated first and only mutate after full rule checks

Lesson:

- item use policy should be centralized in a domain service
- transport handlers should not contain the whole policy tree

## Economy And Service Loops

The source also contains:

- trade and multisell flows
- recipe books and crafting
- personal, freight, and clan warehouse logic
- manor and castle service systems
- mail-related modules

This means economy is not one subsystem. It is a family of flows built on top of the same item and inventory model.

The extracted inventory pass also reinforced two strong concepts to preserve:

- trade and exchange flows should stage and validate before mutating inventory truth
- storage and transfer flows should reuse one shared mutation substrate while still keeping access policy explicit per storage type

## Persistence Map

From the studied files, the most important persistence groups are:

- character core: `characters`
- class template lineage: `class_list`, `char_templates`
- class progression curves: `lvlupgain`
- learned skills: `character_skills`
- saved buffs and reuse: `character_skills_save`
- subclass progression: `character_subclasses`
- henna: `character_hennas`
- shortcuts: `character_shortcuts`
- recommendations: `character_recommends`
- recipes: `character_recipebook`
- friends: `character_friends`
- quests: `character_quests`
- item state: `items`
- item-instance extensions: `items_duration`, `items_period`, `items_attributes`, `items_variation`, `items_options`
- pet state: `pets`
- mission progress: `characters_mission`
- olympiad: `olympiad_data`, `olympiad_nobles`, `olympiad_nobles_eom`
- hero: `heroes`, `heroes_diary`

Clan and alliance related persistence is split between `ClanTable.java`, `L2Clan.java`, and related clan tables such as `clan_data`, rank and sub-pledge persistence, and ranking tables.

## Recommended Aggregate Split For A Rewrite

- `character_profile`
- `character_progression`
- `character_class_state`
- `class_template_catalog`
- `class_progression_curves`
- `character_skillbook`
- `character_loadout`
- `item_template_catalog`
- `item_instance_store`
- `inventory_and_storage`
- `exchange_and_trade_rules`
- `inventory`
- `quest_log`
- `social_profile`
- `competitive_profile`

This is cleaner than letting a single player entity own every table directly.

## Source Anchors

- `java/l2/gameserver/model/base/ClassId.java`
- `java/l2/gameserver/model/base/PlayerClass.java`
- `java/l2/gameserver/templates/CharTemplate.java`
- `java/l2/gameserver/templates/PlayerTemplate.java`
- `java/l2/gameserver/tables/CharTemplateTable.java`
- `java/l2/gameserver/tables/LevelUpTable.java`
- `java/l2/gameserver/model/Player.java`
- `java/l2/gameserver/tables/SkillTable.java`
- `java/l2/gameserver/tables/SkillTreeTable.java`
- `java/l2/gameserver/model/SubClass.java`
- `java/l2/gameserver/model/actor/instance/L2ClassMasterInstance.java`
- `java/net/sf/l2j/gameserver/scriptings/QuestState.java`
- `java/l2/gameserver/data/xml/parser/ItemParser.java`
- `java/l2/gameserver/data/xml/holder/ItemHolder.java`
- `java/l2/gameserver/templates/item/ItemTemplate.java`
- `java/l2/gameserver/model/items/ItemInstance.java`
- `java/l2/gameserver/model/items/ItemContainer.java`
- `java/l2/gameserver/model/items/Inventory.java`
- `java/l2/gameserver/model/items/PcInventory.java`
- `java/l2/gameserver/model/items/Warehouse.java`
- `java/l2/gameserver/network/l2/c2s/UseItem.java`
