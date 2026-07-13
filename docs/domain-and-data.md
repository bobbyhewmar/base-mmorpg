# Domain and Data

## Domain Priorities

Model online play around authoritative character actions, compact world regions, progression, and repeatable city-to-field gameplay loops.

## Core Aggregates

### Account

- player identity
- authentication linkage
- credential and verification state
- login, recovery, and session-security state
- profile and preferences

### Character

- race
- base class
- sex
- normalized unique name
- class or archetype
- stable internal class id, base class lineage, and active subclass index when relevant
- stats and progression
- current HP, MP, and CP stored separately from derived maxima
- location and bound city
- equipment and inventory
- major paperdoll occupancy such as weapon, chest, gloves, and boots in the current slice
- inventory capacity, weight state, and equip-slot occupancy derived from authoritative item placement
- derived visual loadout from equipped gear
- owned companions and active companion slot
- active mount state when mounted
- currency and resources
- active effects and cooldown state
- selected skill or combat-ready action context when needed at runtime
- persistent hotbar bindings and visible hotbar-count preference

### World Region

- region identity and type
- neighboring regions and exits
- geodata version
- navigable terrain surfaces
- static blocking obstacles
- region bounds and portal edges
- spawn groups
- NPC services
- safety or combat rules

### Session

- active connection or presence state
- account-authenticated attach state
- runtime character context
- current region context
- reconnect and logout handling

## Supporting Entities

- registration policies
- character-creation catalogs
- name-validation policies
- hotbar loadout entries
- skill categorization metadata
- NPC templates
- monster templates
- tameable monster profiles
- companion templates
- mount profiles
- terrain or geodata definitions
- region navigation surfaces
- static obstacle definitions
- movement profiles
- class definitions
- class templates
- class progression curves
- attribute bonus curves
- item templates
- item instances
- inventory containers
- storage policies
- exchange recipes or rules
- loot tables
- quests
- equipment templates
- equipment visual variants
- skill definitions
- skill target profiles
- skill timing profiles
- notifications

## Persistence Strategy

Use PostgreSQL tables that keep both the current state and enough history to support audit, support, and abuse investigation.

Suggested initial data groups:

- `accounts`
- `account_credentials`
- `account_verifications`
- `account_recovery_tokens`
- `account_sessions`
- `characters`
- `character_creation_rules`
- `character_name_reservations`
- `character_hotbar_loadouts`
- `character_skill_categories`
- `character_stats`
- `character_locations`
- `character_inventory`
- `equipment_slots`
- `equipment_visual_variants`
- `world_regions`
- `region_connections`
- `region_geodata_versions`
- `region_nav_surfaces`
- `region_static_obstacles`
- `region_portals`
- `movement_profiles`
- `region_spawn_groups`
- `npc_templates`
- `monster_templates`
- `taming_profiles`
- `companion_templates`
- `companion_instances`
- `mount_profiles`
- `class_definitions`
- `class_templates`
- `class_progression_curves`
- `character_subclasses`
- `attribute_bonus_curves`
- `item_templates`
- `vendor_offer_templates`
- `item_instances`
- `inventory_container_membership`
- `equipment_occupancy`
- `item_instance_attributes`
- `item_instance_variations`
- `item_visual_overrides`
- `storage_transfer_records`
- `exchange_rule_sets`
- `loot_tables`
- `skill_definitions`
- `skill_target_profiles`
- `skill_timing_profiles`
- `quest_states`
- `action_logs`
- `notification_intents`
- `email_messages`
- `email_provider_events`
- `jobs`

Add snapshots or compacted action history later when replay, dispute analysis, or rollback tooling justifies it.

## Transaction Boundaries

Keep one transaction per authoritative player action whenever practical. An action should:

1. verify actor, session, and relevant world state
2. apply exactly one legal state transition
3. persist the resulting character, inventory, loot, targeting, or progression effects
4. commit before fan-out or side jobs

Pre-game account and character-entry flows should follow the same bias:

1. verify account or session intent
2. validate registration, login, or character-creation legality on the backend
3. persist exactly one legal account, session, or character mutation
4. commit before notification or email side effects

Combat and skill data should be able to express at least:

- target requirement versus self-cast freedom
- cast-time differences between single-target and area skills
- area-shape and max-target collection
- damage split policy across multiple affected targets
- cooldown and resource cost independently from cast time

Class and progression data should be able to express at least:

- stable class identifiers and parent lineage
- active class versus base class versus subclass slot
- static template seeds separately from HP, MP, and CP progression curves
- explicit per-class, per-level HP, MP, and CP growth curves inspired by the extracted model
- subclass-specific saved current HP, MP, and CP independent from recomputed maxima
- race-linked or sex-linked collision or presentation variants when needed

Character and equipment data should be able to express at least:

- immutable item-template definitions separately from mutable item-instance state
- distinct item-instance identity separate from template identity
- container membership and equip-slot occupancy as authoritative placement state
- stack split and stack merge semantics without ambiguous partial ownership
- storage variants such as personal inventory, equip-capable inventory, private storage, clan storage, pet storage, and freight-like delivery
- template-side flags and conditions separately from runtime transition legality
- which equipped slots affect rendered appearance
- how an item maps to a visible model or variant
- how weapon and armor swaps propagate into the runtime character view
- how default class visuals fall back when a slot is empty
- authoritative buy, sell, and reward valuation without trusting client-supplied prices

Account, session, and character-entry data should be able to express at least:

- account registration and verification state
- login throttling and suspicious-access tracking
- one active gameplay actor binding per accepted online session
- playable races derived from server-owned content
- initial base-class choices grouped under `Fighter` and `Mage`
- sex choice and presentation variant support
- name normalization, profanity filtering, reservation, and uniqueness checks
- character-entry permission separate from mere account authentication

Movement and terrain data should be able to express at least:

- server-owned navigability per region
- static obstacles that block pathfinding
- region exits or portal edges
- actor collision radius or movement-profile constraints
- destination snapping tolerance
- geodata version used by runtime and client reconciliation
- pathfinding budget limits for safe CPU usage

HUD skill and hotbar data should be able to express at least:

- up to `3` visible hotbar rows per character
- `12` slots per row
- stable slot order and slot identity
- bound `skill_id`, `item_instance_id`, `action_id`, or explicit empty state per slot
- backend-owned active or passive skill classification
- persistence of visible hotbar count and slot bindings across disconnect

The current repository slice now concretely exercises this model with:

- `Fighter` and `Mage` class content
- a shipped `dawn_plaza_geo_v1` terrain slice with versioned bounds, static blockers, and deterministic server-side waypoint routing
- learned skills unlocked by class plus level
- passive stat bonuses derived from learned passives
- a persistent `character_hotbar_loadouts` projection returned through `world/enter` and runtime deltas, updated by `set_hotbar_state`
- deterministic item-instance attributes on starter gloves, preserved through authoritative item snapshots and applied only when the item is equipped
- a plaza merchant offer bought through backend-derived pricing with client payload limited to `vendor_offer_id` and `quantity`
- a stackable plaza merchant material bundle bought through the same backend-derived pricing rules, proving vendor handling beyond single non-stackable gear
- a fixed plaza merchant exchange recipe redeemed through backend-derived material requirements with client payload limited to `exchange_offer_id` and `quantity`
- a second plaza exchange recipe that consumes stackable salvage material and returns upgraded boots through the same authoritative exchange path
- inventory-to-currency vendor sales with client payload limited to `item_instance_id` and `quantity`, leaving all valuation on the backend
- a private plaza warehouse keeper that moves item instances between `inventory` and `warehouse` containers through authoritative deposit and withdraw commands
- a minimum runtime-only player-trade offer state between nearby attached sessions, with explicit offer, accept, and decline commands
- authoritative player-to-player inventory trade from one nearby character to another through `offer_trade_item`, `accept_trade_offer`, and `decline_trade_offer`
- persisted `action_logs` rows for the current vendor buy, exchange, sell, warehouse deposit or withdraw, and player-trade offer, accept, decline, send, or receive mutations
- persisted `storage_transfer_records` rows for the current warehouse deposit and withdraw mutations, committed in the same database transaction as the authoritative item move
- actor and command metadata on current audit rows when available, including `character_id`, `account_id`, `session_id`, `command_id`, `command_seq`, quantity, signed currency delta, and cheap before or after snapshots
- token-gated internal read-only investigation endpoints for listing economy events, warehouse transfers, and trade events without exposing a gameplay-facing admin panel

Companion and mount data should be able to express at least:

- which monster species are tameable
- what conditions or items are required to tame them
- whether a tamed creature becomes a combat pet, utility pet, or mount
- stat scaling or inheritance rules from template to persistent companion
- active summon, unsummon, mount, and dismount state
- restrictions on skill usage or actions while mounted

Avoid distributed transactions in the early architecture.

## Concurrency Model

- Serialize conflicting mutations at the character level and at contested-entity boundaries where needed.
- Make idempotency keys explicit for client retries.
- Keep background jobs from competing with live gameplay tables without limits.
- Use optimistic or explicit locking deliberately, not accidentally.
- Prevent duplicate character-name creation and duplicate active-session attachment through explicit backend constraints.

## Data Retention Bias

- Keep operational tables lean.
- Archive or compact deep history later rather than deleting it blindly.
- Keep enough action and economy history to investigate suspicious farming, trades, or progression anomalies.
- Treat auditability as part of game integrity.
