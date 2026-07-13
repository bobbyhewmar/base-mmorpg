# Character, World, And Lifecycle

## Why This Pillar Comes First

Every other system assumes three things already work:

- the character exists and can be restored
- the world knows where that character is
- nearby entities can see, target, and react to that character

If this layer is shaky, combat, quests, PvP, and siege all become ambiguous.

## Actor Hierarchy

The studied source is organized around a strict runtime hierarchy:

- `L2Object` is the identity and world-presence root.
- `Creature` adds movement, combat, effects, stats, status, AI hooks, and death handling.
- `Playable` adds player-or-summon shared behavior.
- `Player` adds persistence, targeting, social state, progression, item use, PvP rules, and most player-specific gates.
- `L2Summon` adds pet and servitor behavior on top of `Playable`.
- `L2Npc` adds NPC behavior on top of `Creature`.
- `Attackable` adds aggro, reward, loot, and raid logic on top of `L2Npc`.

This is the real center of gravity of the game. A new implementation should keep the conceptual split even if the actual type graph is different.

## World Model

`model/L2World.java` is the global registry for:

- all world objects
- online players
- active pets
- the region grid used for visibility and activation

The world is divided into `L2WorldRegion` cells. Entering and leaving a region updates:

- visible object membership
- known lists
- nearby player awareness
- region activation and death handling

Important consequence: the game is not modeled as one giant fully active map. It is a region-based visibility graph.

For our project, this suggests:

- region-scoped update ownership
- local visibility sets
- explicit enter and leave events
- deterministic spawn and despawn rules

## Known List And Visibility

The source uses known lists heavily:

- objects discover nearby objects through region scans
- characters build their own known object sets
- other systems assume targeting only happens against known or visible objects

This is one of the hidden foundations of interaction correctness. It affects:

- target validity
- attack start
- cast range checks
- reward distance checks
- party position updates
- packet fanout

A modern implementation should model visibility as its own domain concern instead of hiding it inside ad hoc utility calls.

## Client Input Entry Points

The main player-intent packets found in the source are:

- `Action.java`
- `MoveBackwardToLocation.java`
- `RequestMagicSkillUse.java`
- `UseItem.java`
- plus many specialized social and territorial packets such as party, pledge, siege, and restart flows

The important flow is:

1. client emits an input packet
2. packet handler resolves the acting player
3. handler validates basic illegal states
4. handler turns the input into target selection, movement, attack, cast, or item use
5. actor and AI layers take over authoritative resolution

For our game, the equivalent domain commands should be explicit:

- `SelectTarget`
- `MoveToGroundPoint`
- `UseSkill`
- `UseItem`
- `InteractWithEntity`
- `RespawnAtPoint`

## Targeting And Interaction

`Action.java` is the first important interaction gate:

- it resolves the clicked object
- blocks interaction in observer and invalid states
- checks certain zone restrictions
- routes left-click to `onAction`
- routes shift-click to `onActionShift`

`Player.onAction` and related actor methods then decide whether the click:

- only selects the target
- triggers an interaction
- begins combat
- is rejected because of event or zone rules

This reinforces a good design rule for us:

- selection and execution are separate concerns
- the server decides whether a selected entity can be acted upon

## Movement

`MoveBackwardToLocation.java` shows the classic click-to-move flow:

- the client sends origin and target coordinates
- the server rejects impossible states
- distance sanity checks prevent absurd movement requests
- the target point is translated into an AI intention

The packet ends with:

- `activeChar.getAI().setIntention(CtrlIntention.MOVE_TO, new Location(...))`

That means movement is not just a coordinate mutation. It is an intention handled by the actor AI layer.

For our compact MMORPG, movement should be modeled as:

- a validated path or intent request
- authoritative position progression on the server
- correction-ready client presentation
- explicit interruption by cast, stun, root, death, or forced reactions

## Character Restore And Persistent Identity

`Player.java` is also the main persistence aggregate in the old source.

The file directly declares SQL for:

- `characters`
- `character_skills`
- `character_skills_save`
- `character_subclasses`
- `character_hennas`
- `character_shortcuts`
- `character_recommends`
- `character_recipebook`
- `character_friends`
- mission and HWID tables

This tells us the original design stores a wide slice of the game straight from the player entity.

Useful lesson:

- the player aggregate is huge in the studied source
- a cleaner rebuild should split identity, loadout, progression, quest log, social data, and live combat state into clearer modules

## Character Lifecycle Hooks

The source shows critical lifecycle hooks on `Player`:

- `onAction`
- `doAttack`
- `doCast`
- `setTarget`
- `onTeleported`
- `addExpAndSp`
- `onKillUpdatePvPKarma`
- death and restart related flows

This means the player object is not passive state. It is an orchestration hub.

In a modern design, move these concerns into services, but preserve the lifecycle order:

1. restore or create character
2. place into world
3. activate visibility
4. allow movement and targeting
5. process combat and interaction
6. handle death or disconnect
7. persist authoritative state

## Zones And State Gating

Many interactions are gated by zone state:

- observer mode
- peace
- PvP
- siege
- raid and boss zones
- event-specific areas

This is spread across packet handlers, `Player`, skills, and event modules in the old source.

For our rebuild, zone rules should become a central policy layer queried by:

- movement
- targeting
- attack
- skill use
- item use
- respawn

## What To Carry Forward

- region-based world ownership
- explicit visibility and known-set logic
- click-to-select and click-to-move as separate actions
- authoritative server movement intentions
- lifecycle hooks for spawn, teleport, death, and reconnect

## What To Avoid Repeating

- direct SQL from the live actor object
- packet handlers calling many unrelated event checks inline
- zone and event rules scattered across transport and actor layers
- making `Player` the dumping ground for every system

## Source Anchors

- `java/net/sf/l2j/gameserver/model/L2World.java`
- `java/net/sf/l2j/gameserver/model/L2WorldRegion.java`
- `java/net/sf/l2j/gameserver/model/actor/Creature.java`
- `java/net/sf/l2j/gameserver/model/actor/Playable.java`
- `java/net/sf/l2j/gameserver/model/actor/Player.java`
- `java/net/sf/l2j/gameserver/network/clientpackets/Action.java`
- `java/net/sf/l2j/gameserver/network/clientpackets/MoveBackwardToLocation.java`
