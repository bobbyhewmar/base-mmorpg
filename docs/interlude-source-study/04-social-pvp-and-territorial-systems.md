# Social, PvP, And Territorial Systems

## Party Is The First Social Combat Unit

`model/L2Party.java` is the first major cooperative aggregate.

It manages:

- membership
- invitations and timeout windows
- loot distribution modes
- party level
- XP and SP sharing
- command channel attachment
- dimensional rift participation
- periodic member position broadcast

Important design lesson:

- party is not just chat membership
- it is part of combat, reward, and navigation logic

If the new game has cooperative grinding, party rules should arrive early.

## Loot And Reward Sharing

Party behavior directly changes:

- who receives drops
- how loot rotation works
- who can legally receive an item
- how XP and SP are scaled with group size

This matters because party policy is a gameplay rule, not a UI preference.

## Command Channel

Command channels appear as a coordination layer above parties.

In the studied source they matter especially for:

- raid participation
- raid looting rights
- larger-scale organization

We should likely defer this until after party is mature, but keep the future expansion path in mind.

## Clan As A Long-Lived Social Aggregate

`model/L2Clan.java` is one of the densest persistence and rule classes in the whole source.

It owns or influences:

- membership
- leader transfer
- alliance links
- crest and ally crest
- clan warehouse
- sub-pledges
- rank privileges
- clan skills
- reputation score
- clan wars
- castle and hideout ownership
- siege statistics
- ranking points in this source

This is not a lightweight guild feature. It is a major game system with economy, territory, and competitive consequences.

## Clan Sub-Units

The source supports:

- academy
- royal guards
- knight units

This is important because clan structure affects:

- hierarchy
- progression
- membership caps
- privileges
- academy or sponsor style loops

For a smaller modern project, this can be deferred, but the data model should not block it forever.

## Clan Reputation And Clan Skills

`L2Clan` contains:

- reputation gain and loss
- pledge skill storage and distribution
- reputation spending for structural upgrades
- side effects on online members when clan state changes

This shows that clan progression is both:

- a meta progression loop
- a live buff or capability provider

## PvP, PK, Karma, And Flagging

The main player combat legality signals in `Player.java` include:

- PvP flag
- karma
- PvP kill count
- PK kill count
- zone-specific hostility handling
- kill-based karma and reward updates

The method `onKillUpdatePvPKarma(...)` is one of the core PvP rule hubs.

Observed behaviors include:

- early exits for PvP-like zones
- war and flagged-target checks
- PvP reward logic in some zones
- karma calculation for unlawful kills
- death penalty interactions

For our project, separate these clearly:

- consensual PvP
- non-consensual hostile action
- criminal or karma consequences
- special event exceptions

## Duel And Similar Competitive Modes

The codebase includes duel handling and many event modes.

That means the source uses a layered competition model:

- open world combat rules
- duel rules
- event-specific combat rules
- olympiad rules
- siege rules

A rewrite should keep those as policies around the combat core rather than branching the combat engine itself everywhere.

## Siege And Territory

`model/entity/Siege.java` is the main territorial warfare controller.

It manages:

- attacker, defender, and waiting clan lists
- registration states
- automatic scheduling
- guard spawning and despawning
- tower spawning
- castle doors
- zone activation
- side-specific teleportation
- mid-siege ownership changes
- post-siege cleanup and rewards

Notable lifecycle:

1. registration opens
2. participants are persisted
3. siege starts
4. attackers are teleported out and defenders hold position
5. towers and doors define the battle state
6. castle owner can change during `midVictory()`
7. siege ends and ownership, rewards, and reputation update

This is effectively a full game mode with long-lived preconditions and postconditions.

## Castle, Clan Hall, And Territory Services

The surrounding territorial layer includes:

- `Castle.java`
- `CastleManager`
- `ClanHallManager`
- `AuctionManager`
- `CastleManorManager`
- doormen, warehouse, teleport, and blacksmith NPC flows

This means territory ownership is not only about combat prestige. It also unlocks service infrastructure.

## Olympiad

`model/olympiad/Olympiad.java` runs the cyclical competitive ladder.

It manages:

- noblesse participants
- competition period versus validation period
- weekly point changes
- season or cycle boundaries
- hero selection preparation
- ranking persistence

It is tightly connected to:

- noblesse eligibility
- class identity
- restricted items and skills
- hero outcomes

This is one of the clearest examples of a system that should be built late, after combat, progression, and social identity are already mature.

## Hero

`model/entity/Hero.java` layers recognition and prestige on top of olympiad.

It tracks:

- current heroes
- all-time heroes
- hero counts
- fights
- diaries
- clan and ally presentation
- castle-taken actions
- hero item cleanup

This is prestige content, not foundational runtime.

## Seven Signs And Other World Meta Systems

The source also includes:

- `SevenSigns`
- `SevenSignsFestival`
- castle manor systems
- Four Sepulchers
- boats and travel networks

These are meaningful to the full Lineage II identity, but they are not prerequisites for a first playable compact MMORPG.

## Recommended Order For Us

1. party
2. basic clan and invitation flows
3. PvP flagging and lawful or unlawful kill rules
4. clan reputation and simple progression
5. territory ownership only after the above is stable
6. olympiad and hero only after the core competitive loop feels strong

## Source Anchors

- `java/net/sf/l2j/gameserver/model/L2Party.java`
- `java/net/sf/l2j/gameserver/model/L2Clan.java`
- `java/net/sf/l2j/gameserver/datatables/ClanTable.java`
- `java/net/sf/l2j/gameserver/model/entity/Siege.java`
- `java/net/sf/l2j/gameserver/model/entity/Castle.java`
- `java/net/sf/l2j/gameserver/instancemanager/SiegeManager.java`
- `java/net/sf/l2j/gameserver/model/olympiad/Olympiad.java`
- `java/net/sf/l2j/gameserver/model/olympiad/AbstractOlympiadGame.java`
- `java/net/sf/l2j/gameserver/model/entity/Hero.java`
- `java/net/sf/l2j/gameserver/instancemanager/SevenSigns.java`
