# Combat, Stats, Skills, And PvE

## Core Combat Stack

The old source resolves combat through a layered stack:

1. the client requests an action such as attack or skill use
2. `Player` validates whether the action is legal right now
3. `Creature` executes attack or cast timing
4. formulas and stats compute hit, crit, speed, damage, reuse, and resist
5. `L2Skill` resolves targets and effects
6. the target loses HP or MP, gains effects, or dies
7. PvE or PvP aftermath logic runs

This is the real heart of the game.

## `Creature` As The Combat Runtime

`model/actor/Creature.java` contains the generic combat engine:

- `doAttack`
- `doCast`
- `reduceCurrentHp`
- `doDie`
- `addEffect`
- `stopAllEffects`
- `calcStat`
- stat and status accessors

Key design lesson:

- live combat state is not just numbers
- it is a state machine around attack timing, cast timing, movement, effects, and death

For our rebuild, combat code should be isolated and deterministic enough to simulate without sockets or a database.

## Stats And Status Split

The source separates stats from current pools:

- `CharStat`, `PcStat`, `PlayableStat`, `SummonStat`
- `CharStatus`, `PcStatus`, `PlayableStatus`, `SummonStatus`
- `Stats` enum for formula keys

This is a useful split:

- stats answer what the character can do
- status answers what the character currently has left

That separation should remain in the new project.

The extracted class-status pass sharpens this further:

- static class seeds come from `PlayerTemplate` through `CharTemplateTable`
- HP, MP, and CP maxima come from `LevelUpTable` plus base-stat multipliers and runtime modifiers
- current HP, MP, and CP are mutable status pools and not the same thing as template data
- derived combat values such as accuracy, evasion, crit, speed, attack, and defense are recomputed through `calcStat()` rather than persisted as permanent truth

## Static Seeds Versus Derived Runtime

The class and stat model is layered:

1. `char_templates.sql` provides most static player-template seeds
2. `lvlupgain.sql` provides HP, MP, and CP curves by exact class id and level
3. `attribute_bonus.xml` provides STR, DEX, CON, INT, WIT, and MEN bonus curves
4. `StatFunctions` adds built-in formula helpers
5. skills, effects, items, augmentations, and XML stat templates inject more funcs
6. `Creature.calcStat()` and `Formulas` produce the effective runtime numbers

Important extracted quirks:

- runtime player templates do not trust SQL `M_DEF`, `M_SPD`, `ACC`, or `EVASION` as live player seeds
- `baseMDef` is hardcoded to `41`
- cast speed seed is injected from config rather than from the player SQL row
- Orcs get a synthesized larger base attack range than other races
- mount state can override movement speed and collision-related behavior

## Skill Model

`model/L2Skill.java` is a large metadata and resolution class.

It contains:

- operation type
- MP cost
- cast range
- effect range
- abnormal level
- target selection
- condition checking
- offensive and debuff classification
- secondary effect templates
- self effect templates
- next-action behavior such as attack chaining

Concrete skill behavior is then implemented through:

- skill subclasses under `gameserver/skills/l2skills`
- handlers and formula logic around skill execution

The key idea is solid even if the implementation is old:

- skill data defines what the skill is
- runtime code defines how it actually resolves

## Target Selection, AoE, And Multi-Target

`L2Skill.getTargetList(...)` is one of the most important methods in the entire source.

It is where the server decides:

- whether the current target is legal
- whether a skill is self, single-target, groundless area, or group oriented
- which entities are actually collected into the final target list

Relevant skill fields include:

- `castRange`
- `effectRange`
- offensive flags
- operate type
- effect templates

This maps directly to our project needs:

- terrain-click movement is one command
- target-click skill execution is another command
- offensive skill execution should resolve from the current valid target unless the skill is explicitly self-centered or ground-targeted
- AoE collection must be fully server-authoritative
- multi-target effects should be represented as target selector policies, not as UI-only logic
- offensive AoE in our project should usually trade burst speed and per-target damage efficiency for broader coverage

## `Player.useMagic` As The Real Rule Gate

The packet `RequestMagicSkillUse.java` mostly resolves the skill and forwards to `Player.useMagic(...)`.

Inside `Player`, the server then checks:

- whether the player knows the skill
- whether the player can currently act
- whether the target is legal for PvP or zone rules
- whether cast conditions and resource costs are satisfied

This is a crucial design clue:

- transport input should remain thin
- gameplay validation belongs in authoritative application and domain layers

## Effects

`L2Skill.getEffects(...)` shows how the source applies secondary effects:

- create environment context
- evaluate success chance
- respect invulnerability and invalid target types
- instantiate effects from templates
- attach effects to the target or self

Effect templates are one of the biggest reasons Lineage-style combat feels rich. Stuns, roots, poisons, toggles, debuffs, self buffs, and procs all build on the same machinery.

In a modern rewrite, effects should become:

- typed rules with explicit stacking policy
- deterministic application and expiration
- separate from raw transport or animation concerns

## PvE Actor Logic

`model/actor/Attackable.java` is the base for monsters, raid bosses, guards, and similar actors.

Its responsibilities include:

- aggro tracking
- damage accounting per attacker
- raid looting rights for command channels
- overhit handling
- reward calculation
- drop generation
- quest kill notifications

The `calculateRewards(...)` path is especially important. It uses:

- total damage dealt
- damage share per attacker
- range checks
- party presence
- summon penalty

That tells us the PvE reward model is not local to the killer alone. It is a distributed resolution step.

## Raid-Specific Signals

The source contains raid-specific behavior such as:

- command channel looting rights
- special reward and restriction logic
- hero and noblesse related consequences in some modules

For our project, raid logic should be designed as a separate policy layer on top of base combat, not woven into every monster path.

## Summons And Pets

The source distinguishes:

- pets
- servitors
- summon-owned penalties and behaviors
- pet item usage and equipment rules

This matters because summoning affects:

- experience penalty and reward share
- target ownership
- item restrictions
- buff and effect propagation

If we support summons, they should be first-class combat actors, not visual attachments to the player.

If we support taming monsters into pets or mounts, treat taming as the acquisition path into the same first-class companion actor model rather than as a special visual exception.

## Hidden Couplings To Watch

- item use restrictions call into combat and event rules
- PvP checks appear in both skill and player logic
- some events inject themselves into target and action flow
- AI, state checks, and formulas are cross-wired through global managers

These are precisely the things to remove in a cleaner rebuild.

## What To Carry Forward

- a deterministic combat state machine
- authoritative target collection
- separate stats and current pools
- separate static class templates from HP, MP, and CP curve tables
- reusable effect machinery
- reward distribution based on contribution, distance, and party context

## What To Simplify

- use clear domain policies for target legality
- isolate formulas from transport and persistence
- keep event-specific combat exceptions out of core skill resolution
- model raid, summon, and PvP variants as policies layered on top of a shared combat core
- model area-skill balance explicitly through cast-time and damage-distribution policies instead of ad hoc per-skill hacks

## Source Anchors

- `java/net/sf/l2j/gameserver/model/actor/Creature.java`
- `java/net/sf/l2j/gameserver/model/L2Skill.java`
- `java/net/sf/l2j/gameserver/model/actor/Player.java`
- `java/net/sf/l2j/gameserver/model/actor/Attackable.java`
- `java/net/sf/l2j/gameserver/network/clientpackets/RequestMagicSkillUse.java`
- `java/net/sf/l2j/gameserver/skills/l2skills/*`
