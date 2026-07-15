# Combat and Targeting

## Direction

Use a classic target-based MMORPG interaction model.

The player moves by clicking a destination point on the terrain, clicks a mob to acquire a combat target, and only then activates offensive skills according to that skill's targeting rule.

The combat reference is Lineage 2 style target locking, not Mu Online style spot-based AoE grinding.

## Core Interaction Rules

### Movement

- Clicking navigable terrain issues a move command toward that world point.
- Movement is target-point based, not freeform WASD as the primary control model.
- The client should start reversible local movement presentation immediately after the click, but the backend remains authoritative for final position, terrain collision, and path validity.
- The client sends the destination point only; it must not send authoritative paths, waypoints, or collision results.
- The backend resolves the destination through server-owned terrain/geodata and returns the accepted route, snapped destination, correction, or rejection.
- The client must blend predicted movement into the authoritative route when the server response arrives instead of making the character wait motionless for pathfinding.
- Movement should automatically route around static obstacles such as rocks, walls, ruins, fences, and cliffs when a legal alternate path exists.
- If no legal route exists, the command should fail with a stable movement reason code instead of letting the client pass through the blocker.

### Targeting

- Clicking a mob targets that mob.
- Offensive engagement starts from a valid current target, not from casting directly into an empty mob spot.
- The current target should remain visible in the HUD until changed, lost, or invalidated by distance or death.
- Non-combat targets such as NPCs, loot, or interactables may use the same target-selection language where helpful.
- Optional `tab` cycling may exist as a convenience, but it only changes the current target. It does not replace explicit target-based combat.
- A known player may be selected authoritatively for the HUD or social commands, but selection alone never makes player damage legal. Player combat runs the additional eligibility contract in `docs/specs/pvp-pk.md`.

### First Player Combat Slice

- `basic_attack` and single-target `use_skill` distinguish player targets from mob targets on the backend, preserving the existing PvE flow.
- Player damage consumes authoritative CP before HP and does not use mob AI, aggro, loot, XP, or mob respawn rules.
- Player attacks validate live attachment, same region, region PvP policy, party and clan relation, life state, range, cooldown, learned skill, and MP.
- Successful hostile damage starts or refreshes a temporary backend-owned PvP flag.
- Player death classifies one durable PvP kill or PK consequence before the existing simple respawn.
- Player AoE, auto-approach, auto-repeat, pet attribution, war, siege, olympiad, rewards, and complex penalties remain deferred.

### Skill Activation

- Single-target offensive skills require a current valid target before activation.
- Target-centered offensive area skills also require a current valid target before activation.
- Self skills and self-centered skills may activate without a hostile target when their rules allow it.
- Ground-point targeting should be rare and explicit, not the default combat language.
- Area skills apply around the caster or the current target when the skill definition says so.
- Some skills affect multiple valid targets in a bounded rule set.
- Offensive area skills should generally cast more slowly than single-target damage skills.
- Offensive area skills should generally distribute their damage budget across the affected targets instead of dealing full single-target damage to each one.

Do not model the combat loop around free AoE spam into farming spots.

### Mob Personality And Aggro

- Monster templates must declare a canonical personality: `passive` or `aggressive`.
- Passive monsters never initiate combat only because the player walked near them.
- Passive monsters enter `aggro` after receiving damage, then follow the same chase and attack loop as hostile monsters.
- Aggressive monsters detect the player within their aggro radius, enter `aggro` immediately, pursue through server-authoritative movement, and attack only when inside their attack range.
- Mob attacks must be backend-owned. The client may display `ai_state`, movement, and hit feedback, but it must not decide aggro, chase legality, attack cadence, or damage.
- Ranged player skills should not cause impossible instant melee retaliation from outside mob attack range; the mob should chase until it can legally attack.

### Companion And Mount Combat

- Tameable monsters should become persistent companion actors, not one-off visual flags on the player.
- Combat pets should follow the same explicit targeting language as the player-facing combat model.
- Pet attacks or pet skills should resolve from a valid target selection or an equally explicit pet-command rule, never from fuzzy autonomous spot farming.
- Mounting should not replace the primary control model: the player still clicks terrain to move and clicks entities to target.
- Mounted state may restrict skill use, attack types, or interaction verbs, but those restrictions must be explicit in the skill or mount rules.
- Taming, summoning, unsummoning, mounting, and dismounting should all be modeled as authoritative state transitions.

## Targeting Taxonomy

Model skills using explicit target semantics rather than ad hoc exceptions.

Suggested categories:

- `self`
- `single_target_enemy`
- `single_target_ally`
- `target_centered_aoe`
- `self_centered_aoe`
- `ground_targeted_aoe`
- `multi_target_chain`
- `cone_or_directional`

The first implementation should prioritize `single_target_enemy`, `target_centered_aoe`, and `self_centered_aoe`.
Ground-targeted skills may exist later, but they are not the default offensive interaction style.

## Validation Rules

The backend should validate:

- line of authority for the actor and character
- skill availability and cooldown
- mana or resource cost
- cast time and commitment rules
- target existence and relation
- range and region legality
- authoritative terrain/geodata navigability for movement
- server-side route generation and obstacle avoidance for move commands
- blocked, unreachable, or snapped movement destinations
- whether the selected skill requires a current target and whether that target is still valid
- area-of-effect target collection
- per-target damage split when the skill uses distributed area damage
- hit or effect resolution order
- companion ownership and active-companion legality when pet actions are involved
- mount-state restrictions before allowing mounted skills, attacks, or interactions

## Client Implications

- The scene must support ground clicks distinctly from entity clicks.
- Movement prediction must be visually reversible until the backend returns an authoritative route or rejection.
- The local player should begin moving immediately on click in normal online play.
- Predicted routes and accepted server routes should be visually distinguishable when both are shown.
- Reconciliation should usually be a smooth blend to the server route, not a hard stop.
- Blocked or unreachable terrain should produce clear HUD or world-space feedback.
- The HUD must clearly show the current target and selected skill state.
- The HUD must clearly show active companion or mount state when one is present.
- Target-required skills should feel unavailable until a valid target is selected.
- Target-centered and self-centered area skills need readable preview shapes or affected-target highlights when useful.
- Ground-targeted skills need visible previews before cast confirmation or dispatch when they exist.
- Companion commands, tame eligibility, and mount restrictions should feel explicit instead of hidden behind animation or flavor.
- Active skills should be bindable into the skill hotbar surface, while passive skills remain visible in the skill-book view without pretending to be triggerable actions.
- Skill-book categorization should separate `Active` from `Passive` using backend-owned classification, not ad hoc client heuristics.
- Cooldown readability should be expressed directly on the skill icon through a top-to-bottom clearing overlay so the player can read reuse state without opening extra panels.

## Data Implications

Skill definitions should carry at least:

- target type
- target requirement policy
- cast range
- radius, width, or chain count when relevant
- max target count when relevant
- selection or preview rules
- cast time profile
- damage distribution policy
- effect application order if it matters

Movement and terrain definitions should carry at least:

- region geodata version
- navigable surface or cell definitions
- static obstacle definitions
- actor collision or movement profile
- destination snapping tolerance
- pathfinding budget limits

Action logs should capture enough context to inspect disputes:

- actor character
- selected skill
- target entity if any
- target point if any
- resolved affected targets
- result summary

## Prototype Recommendation

The first meaningful combat slice should support:

1. click-to-move on terrain
2. click-to-target mob
3. one basic single-target skill
4. one target-centered or self-centered area skill with slower cast and split damage
5. one multi-target or chain-style skill that still resolves from a valid combat target when appropriate

This is enough to prove the target-based interaction language before the skill catalog expands.
