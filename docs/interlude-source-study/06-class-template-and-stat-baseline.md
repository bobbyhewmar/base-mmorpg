# Class Template And Stat Baseline

This document consolidates the extracted class-status pass from the studied source so we can understand its class and stat model, then re-express the useful concepts in our own implementation.

## Highest-Value Findings

- `PcTemplate` does not exist in this branch; the runtime player-template object is `PlayerTemplate`, which extends `CharTemplate`.
- Static player template data is SQL-backed through `char_templates.sql`, not XML-backed.
- Runtime player templates are built by `CharTemplateTable`, which joins `class_list` and `char_templates`.
- HP, MP, and CP maxima do not come from the player template for players; they come from `lvlupgain.sql` through `LevelUpTable`.
- Five playable races exist in this branch: `Human`, `Elf`, `Dark Elf`, `Orc`, and `Dwarf`.
- `ClassId` is the canonical runtime identity tree, while `PlayerClass` carries important subclass-policy behavior.
- Several SQL columns exist but are ignored by the runtime player loader, including `M_DEF`, `M_SPD`, `ACC`, `EVASION`, `canCraft`, and several unknown fields.
- The loader also synthesizes baseline values such as `baseMDef = 41`, cast speed from config, attack range by race, and tiny regen seeds.
- Current HP, MP, and CP are mutable pools stored separately from derived maxima.
- Subclass persistence stores current pools per subclass and restores those pools on subclass switch.
- Fourth classes often reuse earlier static STR, DEX, CON, INT, WIT, and MEN baselines while still getting distinct HP, MP, and CP curves through `lvlupgain`.

## Source Anchors

- `java/l2/gameserver/model/base/ClassId.java`
- `java/l2/gameserver/model/base/PlayerClass.java`
- `java/l2/gameserver/model/base/Race.java`
- `java/l2/gameserver/model/base/ClassType2.java`
- `java/l2/gameserver/templates/CharTemplate.java`
- `java/l2/gameserver/templates/PlayerTemplate.java`
- `java/l2/gameserver/tables/CharTemplateTable.java`
- `java/l2/gameserver/tables/LevelUpTable.java`
- `java/l2/gameserver/model/Creature.java`
- `java/l2/gameserver/model/Playable.java`
- `java/l2/gameserver/model/Player.java`
- `java/l2/gameserver/model/base/BaseStats.java`
- `java/l2/gameserver/stats/Stats.java`
- `java/l2/gameserver/stats/StatFunctions.java`
- `java/l2/gameserver/stats/Formulas.java`
- `dist/gameserver/sql/install/class_list.sql`
- `dist/gameserver/sql/install/char_templates.sql`
- `dist/gameserver/sql/install/lvlupgain.sql`
- `dist/gameserver/data/attribute_bonus.xml`
- `dist/gameserver/config/altsettings.properties`
- `dist/gameserver/config/formulas.properties`

## Identity Model

Class identity is not stored in one place.

- `ClassId` defines numeric class id, parent lineage, race, mage flag, tier, and subclass-certification family.
- `PlayerClass` defines important subclass exclusion families and availability policy.
- `class_list.sql` mirrors class ids and parent relationships for loader use, but runtime lineage checks are driven primarily by `ClassId`.
- `ClassId.getLevel()` is a one-based profession tier, while `char_templates.level` is a separate zero-based SQL field.
- `PlayerClass.values()[classId]` is used directly in subclass code, which makes enum order part of the behavior contract.

## Template Load Path

The static player-template path is:

1. load class lineage metadata from `class_list.sql`
2. load static baseline rows from `char_templates.sql`
3. materialize runtime `PlayerTemplate` instances in `CharTemplateTable`
4. create male and female runtime templates from one SQL row
5. key the female template under `classId | 256`
6. fetch per-class, per-level HP, MP, and CP values later through `LevelUpTable`

This means the new project should not collapse:

- class identity
- static class template
- HP, MP, and CP progression curves
- current HP, MP, and CP pools

into one flat blob, even if we redesign the concrete tables, identifiers, or loaders.

## Loaded Fields And Loader Quirks

### Loaded directly from `char_templates.sql`

- class id and class name
- race
- STR, DEX, CON, INT, WIT, MEN
- base PAtk, PDef, and MAtk
- physical attack speed
- critical rate
- run speed and walk speed
- male and female collision size
- spawn location
- starter items

### Not loaded directly even though SQL columns exist

- `M_DEF`
- `M_SPD`
- `ACC`
- `EVASION`
- `canCraft`
- `M_UNK1`
- `M_UNK2`
- `F_UNK1`
- `F_UNK2`
- SQL `parent`

### Synthesized or hardcoded by the loader

- `baseMDef = 41`
- cast speed seed from `BaseMageCastSpeed` or `BaseWarriorCastSpeed`
- base attack range `20` for most races and `25` for Orcs
- HP, MP, and CP regen seeds of `0.01`
- base shield values of `0`

## HP, MP, And CP Model

Player max HP, MP, and CP do not come from static player-template constants.

- `LevelUpTable` reads `lvlupgain.sql` by exact `class_id` and `level`
- `Creature.getMaxHp()`, `getMaxMp()`, and `getMaxCp()` recompute effective maxima through `calcStat()`
- base attributes such as `CON` and `MEN` still matter because they feed multiplier curves
- fourth classes can reuse the same static template row as earlier classes while still getting distinct HP, MP, and CP curves through their own `class_id`

Current HP, MP, and CP are separate mutable pools:

- runtime pools live as `_currentHp`, `_currentMp`, and `_currentCp`
- subclass persistence stores `curHp`, `curMp`, and `curCp`
- `character_subclasses.maxHp`, `maxMp`, and `maxCp` are written but are not the restore-time source of truth

## Formula Pipeline

The effective stat pipeline is:

1. static class template seeds from `PlayerTemplate`
2. HP, MP, and CP curve input from `LevelUpTable`
3. attribute multiplier curves from `attribute_bonus.xml` through `BaseStats`
4. built-in funcs from `StatFunctions`
5. additional funcs from skills, effects, items, augmentations, and XML stat templates
6. final stat and damage resolution in `Creature.calcStat()` and `Formulas`

Important implications:

- derived combat stats are not persisted as permanent truth
- accuracy and evasion are formula outputs, not player-template seeds in this branch
- movement speed can be overridden by mount state
- collision can be overridden by transformation state

## Recalculation Hooks

The extracted pass confirmed these major recalculation events:

- level up
- class transfer through `Player.setClassId()`
- subclass addition, replacement, and switch
- skill learn, auto-learn, and skill restore
- equip and unequip
- passive skill attach or detach
- effect start and exit
- henna restore
- weight or expertise penalties
- mount state change
- transformation state change

## Static Template Groups

The source effectively uses nine repeating static template groups in `char_templates.sql`.
Exact HP, MP, and CP still remain per-class curves in `lvlupgain.sql`.

### Human Fighter Group

- Classes: `0 Human Fighter`, `1 Warrior`, `2 Gladiator`, `3 Warlord`, `4 Human Knight`, `5 Paladin`, `6 Dark Avenger`, `7 Rogue`, `8 Treasure Hunter`, `9 Hawkeye`, `88 Duelist`, `89 Dreadnought`, `90 Phoenix Knight`, `91 Hell Knight`, `92 Sagittarius`, `93 Adventurer`
- Base attributes: `STR 40`, `DEX 30`, `CON 43`, `INT 21`, `WIT 11`, `MEN 25`
- Move speed: `run 115`, `walk 80`
- Physical attack speed: `330`
- Cast seed: config cast speed `333`
- Crit seed: `44`
- Collision: `M 9.0 x 23.0`, `F 8.0 x 23.5`
- Notes: attack range `20`; MDef hardcoded `41`

### Human Mage Group

- Classes: `10 Human Mage`, `11 Human Wizard`, `12 Sorcerer`, `13 Necromancer`, `14 Warlock`, `15 Cleric`, `16 Bishop`, `17 Human Prophet`, `94 Archmage`, `95 Soultaker`, `96 Arcana Lord`, `97 Cardinal`, `98 Hierophant`
- Base attributes: `STR 22`, `DEX 21`, `CON 27`, `INT 41`, `WIT 20`, `MEN 39`
- Move speed: `run 120`, `walk 78`
- Physical attack speed: `303`
- Cast seed: config cast speed `333`
- Crit seed: `40`
- Collision: `M 7.5 x 22.8`, `F 6.5 x 22.5`
- Notes: attack range `20`; MDef hardcoded `41`

### Elf Fighter Group

- Classes: `18 Elf Fighter`, `19 Elf Knight`, `20 Temple Knight`, `21 Swordsinger`, `22 Scout`, `23 Plains Walker`, `24 Silver Ranger`, `99 Eva Templar`, `100 Sword Muse`, `101 Wind Rider`, `102 Moonlight Sentinel`
- Base attributes: `STR 36`, `DEX 35`, `CON 36`, `INT 23`, `WIT 14`, `MEN 26`
- Move speed: `run 125`, `walk 90`
- Physical attack speed: `345`
- Cast seed: config cast speed `333`
- Crit seed: `46`
- Collision: `M 7.5 x 24.0`, `F 7.5 x 23.0`
- Notes: attack range `20`; MDef hardcoded `41`

### Elf Mage Group

- Classes: `25 Elf Mage`, `26 Elf Wizard`, `27 Spellsinger`, `28 Elemental Summoner`, `29 Oracle`, `30 Elder`, `103 Mystic Muse`, `104 Elemental Master`, `105 Eva Saint`
- Base attributes: `STR 21`, `DEX 24`, `CON 25`, `INT 37`, `WIT 23`, `MEN 40`
- Move speed: `run 122`, `walk 85`
- Physical attack speed: `312`
- Cast seed: config cast speed `333`
- Crit seed: `41`
- Collision: `M 7.5 x 24.0`, `F 7.5 x 23.0`
- Notes: attack range `20`; MDef hardcoded `41`

### Dark Elf Fighter Group

- Classes: `31 DE Fighter`, `32 Palus Knight`, `33 Shillien Knight`, `34 Bladedancer`, `35 Assassin`, `36 Abyss Walker`, `37 Phantom Ranger`, `106 Shillien Templar`, `107 Spectral Dancer`, `108 Ghost Hunter`, `109 Ghost Sentinel`
- Base attributes: `STR 41`, `DEX 34`, `CON 32`, `INT 25`, `WIT 12`, `MEN 26`
- Move speed: `run 122`, `walk 85`
- Physical attack speed: `342`
- Cast seed: config cast speed `333`
- Crit seed: `45`
- Collision: `M 7.5 x 24.0`, `F 7.0 x 23.5`
- Notes: attack range `20`; MDef hardcoded `41`

### Dark Elf Mage Group

- Classes: `38 DE Mage`, `39 DE Wizard`, `40 Spell Howler`, `41 Phantom Summoner`, `42 Shillien Oracle`, `43 Shillien Elder`, `110 Storm Screamer`, `111 Spectral Master`, `112 Shillen Saint`
- Base attributes: `STR 23`, `DEX 23`, `CON 24`, `INT 44`, `WIT 19`, `MEN 37`
- Move speed: `run 122`, `walk 85`
- Physical attack speed: `309`
- Cast seed: config cast speed `333`
- Crit seed: `41`
- Collision: `M 7.5 x 24.0`, `F 7.0 x 23.5`
- Notes: attack range `20`; MDef hardcoded `41`

### Orc Fighter Group

- Classes: `44 Orc Fighter`, `45 Raider`, `46 Destroyer`, `47 Monk`, `48 Tyrant`, `113 Titan`, `114 Grand Khauatari`
- Base attributes: `STR 40`, `DEX 26`, `CON 47`, `INT 18`, `WIT 12`, `MEN 27`
- Move speed: `run 117`, `walk 70`
- Physical attack speed: `318`
- Cast seed: config cast speed `333`
- Crit seed: `42`
- Collision: `M 11.0 x 28.0`, `F 7.0 x 27.0`
- Notes: attack range `25`; MDef hardcoded `41`

### Orc Mage Group

- Classes: `49 Orc Mage`, `50 Shaman`, `51 Overlord`, `52 Warcryer`, `115 Dominator`, `116 Doomcryer`
- Base attributes: `STR 27`, `DEX 24`, `CON 31`, `INT 31`, `WIT 15`, `MEN 42`
- Move speed: `run 121`, `walk 70`
- Physical attack speed: `312`
- Cast seed: config cast speed `333`
- Crit seed: `41`
- Collision: `M 7.0 x 27.5`, `F 8.0 x 25.5`
- Notes: attack range `25`; MDef hardcoded `41`

### Dwarf Fighter Group

- Classes: `53 Dwarf Fighter`, `54 Scavenger`, `55 Bounty Hunter`, `56 Artisan`, `57 Warsmith`, `117 Fortune Seeker`, `118 Maestro`
- Base attributes: `STR 39`, `DEX 29`, `CON 45`, `INT 20`, `WIT 10`, `MEN 27`
- Move speed: `run 115`, `walk 80`
- Physical attack speed: `327`
- Cast seed: config cast speed `333`
- Crit seed: `43`
- Collision: `M 9.0 x 18.0`, `F 5.0 x 19.0`
- Notes: attack range `20`; MDef hardcoded `41`; SQL `canCraft` flag ignored by loader

## What This Means For Our Project

- We should model class identity, static class template, HP or MP or CP curve tables, and current subclass pools as separate concerns.
- We should not pretend that all baseline stats are on one class row.
- We should preserve the separation between class identity, template seeds, growth curves, and mutable pools even if our own ids, files, or schemas differ.
- We should not carry over branch-specific enum coupling, ignored legacy columns, or loader quirks unless we deliberately decide they help our game.
- We should treat current HP, MP, and CP as mutable session state tied to the active class or subclass slot.
- We should keep rebalancing explicit and layered on top of the extracted model instead of silently rewriting the underlying class concepts.
