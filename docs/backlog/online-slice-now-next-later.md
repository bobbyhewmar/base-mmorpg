# Online Slice Now / Next / Later

## Objective

Keep the online backlog aligned with the code that already exists in the repository and with the execution order frozen by `TRAE_SOLO_MASTER_PROMPT.md`.

This document now distinguishes between:

- what is already implemented in the first authoritative online slice
- what is partially implemented and still below public-readiness
- what comes next without reopening authority boundaries

## Current Status

The first authoritative online slice is no longer only planned. It is implemented end to end and ready to advance to the next roadmap phase.

### Already Implemented

The repository already contains these online capabilities:

- registration and login
- pending-verification and recovery UI hooks
- character list retrieval
- authoritative character creation by race, base class, sex, hairstyle, hair color through canonical `hair_color`, skin type through `skin_type`, and name
- `POST /v1/world/enter`
- durable gameplay-session records
- durable per-character session ownership with server instance id, renewable lease, monotonic fencing token, and conditional release
- WebSocket attach and `region_context`
- authoritative command envelope and command lifecycle
- revision and region-revision handling
- movement with authoritative backend acceptance and client-side presentation
- `select_target`
- `use_skill`
- runtime cooldown handling
- mob death and respawn
- player death and simple respawn
- explicit respawn checkpoint persistence across disconnect and restart for the current simple death flow
- loot spawn, pickup, contention handling, and regional loot fan-out
- persistent inventory and equipment
- bcrypt-backed password storage with legacy SHA-256 upgrade on successful login
- durable account access sessions with expiry in PostgreSQL-backed mode
- minimum rate limiting on auth and attach entry points
- configurable HTTP CORS handling for direct cross-origin access
- durable gameplay command dedup keyed by `session_id + command_seq`
- browser E2E over Docker Compose for bootstrap, movement, target, skill, loot, and equip/unequip
- derived equipment stats in `world/enter.self_state` and `delta.self.stats`
- authoritative player HP in `self_state` and `delta.self`
- minimum defense-based mitigation on incoming player damage
- durable HP/MP, XP/level, and cooldown end timestamps reflected through `world/enter`, attach-time runtime hydration, and restart-safe PostgreSQL state
- minimum structured observability with `/metrics`, request latency, command latency, attach counters, outbound message counters, reject counters by `reason_code`, active socket gauges, attached-session gauges, region occupancy, and persistence error counters
- PostgreSQL-backed accounts, credentials, characters, sessions, position, region, and item state when `L2BG_DATABASE_URL` is configured
- multiplayer player presence between sessions, including in-region player appear, movement fan-out, and disconnect cleanup
- minimal cross-instance presence classification that distinguishes ready local, remote-online, and offline without persisting `known-set`
- PostgreSQL gameplay outbox with monotonic event ids, exact-instance claiming, immutable idempotency keys, retry/dead-letter state, delivered-only retention, and structured lifecycle observability
- replay-safe remote-target notice from one instance to the current target owner while the originating command still rejects with `presence.target_remote`
- replay-safe remote whisper plus party/clan lifecycle notices, with exact-session-and-fence ownership validation, durable delivery/consume receipts, bounded server/client duplicate suppression, authoritative social-state rehydration, and explicit stale-owner retry/dead-letter
- exact-recipient same-region player visual projections through the PostgreSQL outbox, with source/recipient ownership revalidation, monotonic fence/version ordering, heartbeat snapshots, bounded/coalescing publication, durable supersession/compaction of obsolete undelivered snapshots, position-anchor-based remote interest filtering, despawn, TTL, delivery-pressure metrics, and existing browser interpolation
- a separate two-backend Docker Compose profile and Playwright fault/load scenario covering bidirectional projection/region chat, stop/restart, retries, receipts, stale owner, tombstone non-resurrection, TTL, recovery, and measured outbox delay/volume
- backend-derived `next_command_seq` hydration preserves the durable replay namespace when reconnect reuses an existing gameplay session
- command-driven party/clan mutation, command outcome, and remote outbox intents committed atomically, with local fanout deferred until commit
- class-specific learned skills and active or passive categorization
- authoritative skill book projection and persistent hotbar snapshot in `world/enter` and runtime deltas
- local and online HUD hotbar rendering from authoritative loadout instead of global hardcoded skill buttons
- classic compact HUD presentation for player frame, target frame, chat, hotbar, inventory, and skill book
- `ALT+V` inventory window closed by default, with icon-only item grid, footer currency readout, future weight readout, and working title-bar close control
- `ALT+K` skill-book window with `Active` and `Passive` tabs, `32x32px` icon grid, hover/focus tooltips, and active-skill drag to hotbar through authoritative `set_hotbar_state`
- `ALT+V` inventory item drag and `ALT+C` action drag to the shortcut/action bar through authoritative `set_hotbar_state`
- additional equipment slots beyond only weapon and chest, with authoritative equip or unequip through the online path
- authoritative inventory stack split and merge through command deltas instead of client-owned mutation
- deterministic item-instance attributes flowing through persistence, deltas, online read-model projection, and HUD presentation
- class-specific starter gear is now separated between physical `Fighter` packs and mystic `Mage` packs, including backend stats, equip legality, vendor values, and client templates
- player and other-player class body visuals now use Universal Base Characters `gltf_base_character` runtime assets under `src/assets/characters/universal-base`, with Universal Animation Library clips and no procedural character fallback while the canonical asset loads
- Modular Character Outfits - Fantasy assets are kept as future equipment visuals and must not be used as base class bodies
- `Medieval Village MegaKit[Standard]` is available in `3DAssets` as the preferred future medieval map kit; the active clean prototype region uses only selected visual ground/low-vegetation runtime modules under `src/assets/maps/medieval-village-megakit`, and future map slices should publish additional modules to `src/assets/maps` only module-by-module when they update the full map/geodata contract
- authoritative vendor purchase through `buy_item`, with backend-derived pricing and merchant-range validation
- authoritative fixed merchant exchange through `exchange_item`, with backend-derived material requirements and reward validation
- authoritative vendor sale through `sell_item`, with backend-derived valuation and inventory-only legality
- authoritative private warehouse deposit and withdraw through dedicated storage commands and shared item-instance container truth
- authoritative player-to-player trade with explicit offer, accept, and decline between nearby attached sessions
- authoritative quest persistence with reconnect-safe snapshot hydration
- authoritative NPC interaction services for quest acceptance, quest turn-in, merchant access, and warehouse access
- authoritative `use_item` for consumables from inventory and hotbar shortcuts
- authoritative pets, taming, summon or dismiss, and mount or dismount in a first vertical slice with persistent ownership and backend-owned mounted move speed
- authoritative canonical-minimum party invites, membership, leave or kick, and compact roster projection in a first social slice
- authoritative social chat through `send_chat_message`, with `region`, `party`, `alliance`, and `whisper` fan-out plus minimum persisted history and cross-instance exact-recipient delivery for remote party members
- minimum authoritative party reward sharing, including same-party online/attached/alive XP split and party-owned loot pickup eligibility

### Implemented But Still Incomplete For Public Readiness

Remaining work:

- richer economy variants beyond the current merchant, warehouse, and trade vertical slices
- broader social systems on top of the stable party, chat, shared XP, and party-owned loot foundation
- pet combat, pet inventory, pet equipment, breeding, and broader companion AI beyond the first authoritative slice

## Agora

The current execution priority should follow the master prompt and the real repository state:

1. iterate on measured interest-anchor accuracy and broader cross-instance social fanout on top of the shipped safe supersession/compaction for obsolete undelivered regional projection snapshots and remote party chat, without remote combat or new infrastructure
2. keep the shipped PvP/PK transaction and attribution audit under multi-actor load, then deepen karma recovery, account/device correlation, and alerting without automatic punishment
3. instances, siege, olympiad, and broader competitive systems only after ownership, cross-instance presence delivery, PvP/PK, and clan base remain stable

### Fase A - Consolidacao Imediata

Focus:

- remove outdated docs
- align backlog wording with shipped code
- add an explicit readiness checklist for the current online slice
- align skill guidance with the master prompt
- reflect that authoritative alliance foundation and the first authoritative alliance-chat slice are now shipped while command channel, siege, and broader political systems remain deferred

## Depois

### Fase D - E2E Docker Online

Focus:

- automate register/login/create/enter/attach
- automate movement, targeting, combat, loot, and equipment
- document how to run the suite locally

Status:

- concluida para o fluxo online atual

### Fase E - Observabilidade Minima

Focus:

- structured logs
- request and command latency
- counters by `reason_code`
- session, attach, region, and DB error visibility

Status:

- concluida para o slice online atual
- hardening multi-instancia concluido para ownership: attach serializado por personagem no PostgreSQL, lease renovavel, fencing monotono, stale-owner reject antes de ack/dedup e release idempotente/condicional
- presence cross-instance minima distingue player local, remote-online e offline; outbox PostgreSQL entrega notice de target, whisper remoto, region chat, party chat, notices party/clan e projecao visual versionada de player/movimento na mesma regiao com filtro remoto por ancora de posicao autoritativa, enquanto combate remoto e replicacao autoritativa continuam pendentes

### Fase F - Persistencia de Progressao Online

Focus:

- durable HP/MP
- durable XP/level
- durable cooldown end timestamps
- durable death or respawn checkpoints where needed
- restart-safe `world/enter` behavior for these fields

Status:

- concluida para o slice online atual

### Fase G - Multiplayer Presence Real

Focus:

- player entity appear/disappear
- movement updates for other players in the same region
- presence cleanup on disconnect or close
- region occupancy metrics

Status:

- concluida para o slice online atual

### Fase H - Conteudo de classe/skill/hotbar

Focus:

- class definitions and template stats
- active and passive skill definitions
- learned-skill unlocking rules by class and level
- persistent hotbar state
- authoritative HUD skill book and hotbar projection
- classic HUD skill-book and hotbar visuals with `32x32px` slots
- authoritative `skill`, `item`, and `action` drag-to-hotbar persistence through `set_hotbar_state`
- classic bottom-right quick access mini menu for Status, Inventory, Map, and System with `32x32px` icon buttons and classic confirmation modal behavior

Status:

- concluida para o slice online atual
- `use_item` autoritativo agora cobre consumiveis no inventario e na hotbar
- actions futuras alem de `basic_attack`/`pick_up_nearby` continuam como expansao separada
- bottom-right quick access mini menu implemented with `ALT+T`, `ALT+V`, `ALT+M`, `ALT+X`, classic map placeholder, system menu and exit confirmation
- top-right minimap now projects player-centered region position and camera-direction arc from runtime state; `ALT+M` uses the same position/yaw projection in a classic draggable map window instead of remaining static

### Fase I - Inventario e economia ampliados

Focus:

- additional equipment slots
- item attributes
- stack split or merge
- vendors and backend-derived pricing

Status:

- em andamento
- additional slots are now implemented for gloves and boots
- stack split and merge are now implemented with authoritative inventory deltas
- item attributes are now implemented in a first vertical slice through deterministic starter-glove bonuses
- vendor buy is now implemented in a first vertical slice with one plaza merchant offer and backend-derived pricing
- vendor buy now also ships a stackable salvage-material bundle, proving the same authoritative pricing flow for stackable non-currency inventory
- fixed merchant exchange is now implemented in a first vertical slice with one plaza recipe and authoritative material consumption
- fixed merchant exchange now also ships a shard-driven boots recipe, proving stackable-material consumption and upgraded-equipment rewards through the same authoritative path
- vendor sell is now implemented in a first vertical slice with one plaza merchant deriving sell value from the backend
- private warehouse deposit and withdraw are now implemented in a first vertical slice with one plaza vaultkeeper and authoritative container transfers
- warehouse coverage now explicitly includes partial storage and retrieval of stackable non-currency material, not only Duskgold
- a minimum persisted economy audit trail now exists for the shipped vendor and warehouse mutations
- minimum persisted economy auditability now exists through `action_logs` for vendor buy, exchange, and sell plus `storage_transfer_records` for warehouse deposit and withdraw
- player-to-player trade is now implemented in a first vertical slice with explicit consent, proximity validation, and atomic inventory mutation
- player-trade auditability now exists through paired `action_logs` rows for sender and recipient
- economy auditability is now expanded with stable `action_type` coverage for vendor, exchange, warehouse, and trade lifecycle events plus actor/account, quantity, before/after, currency delta, and command metadata where available
- a token-gated internal read-only investigation surface now exists through `/internal/economy/events`, `/internal/economy/warehouse-transfers`, and `/internal/economy/trades`

### Fase I.5 - Server Terrain, Geodata e Pathfinding

Focus:

- server-owned region geodata
- static blockers for rocks, walls, ruins, fences, cliffs, and similar obstacles
- backend pathfinding that generates alternate routes around blockers
- destination snapping or rejection with stable movement reason codes
- authoritative route output through the existing command lifecycle
- immediate browser movement prediction plus pending versus accepted routes
- smooth blend from local prediction to authoritative server path

Status:

- concluida para o slice online atual
- `stonecross_plaza` currently ships as a compatibility region id backed by `clean_plain_1024_geo_v1`: a clean 1024x1024 playable plain with deterministic pathfinding, bounded cancellation, immediate local prediction, prediction leash, and smooth browser reconciliation from pending to authoritative route
- authored Stonecross city content has been removed from the active map: no initial city, NPC services, mobs, buildings, props, terrain overlays, roads, water, grind zones, or spawn packs should be assumed
- `clean_plain_1024_geo_v1` has no authoritative obstacles; the entire 1024x1024 region is walkable until the next approved map concept introduces blockers deliberately
- local retro GLB map assets remain preserved as a content library, but are not active map placements
- renderer, ground raycast/picking plane, spawn/checkpoint, server geodata bounds, and tests must stay synchronized to the same map bounds; no old-map client clamp is allowed
- there are no authored blockers in the active clean region
- `dawn_plaza` may remain only as an id alias to the same clean 1024x1024 geodata for older persisted characters; it must not carry old bounds, blockers, or spawns

Delivered implementation shape:

- keep `move_intent` destination-point based
- reject client-supplied paths, waypoints, collision results, or geodata overrides
- start local predicted locomotion immediately after terrain click/dispatch
- use a prediction leash to avoid large visual drift before authority arrives
- blend predicted locomotion to the authoritative route when the server responds
- keep route state in runtime memory, not as per-frame PostgreSQL writes
- make the pathfinding core deterministic and testable without network or database dependencies
- keep backend pathfinding bounded, cancelable, and non-blocking from the player's point of view
- add integration or E2E coverage for moving around a blocker and failing against an unreachable destination
- add client or E2E coverage proving click-to-move begins locally before the authoritative route response

### Fase J - Quests e NPC services

Focus:

- quest definitions
- persistent quest state
- authoritative NPC interaction commands
- authoritative rewards
- client-projected dialog with backend-owned quest truth
- quest tracker only when real quest data exists

Status:

- concluida para o slice online atual
- quest state rehydrates through `world/enter.self_state.quest`
- NPC interactions project merchant, warehouse, and wardkeeper dialog states from authoritative snapshots
- quest accept and turn-in remain replay-safe and do not duplicate rewards

### Fase K - Pets, taming e mounts

Focus:

- tameable monster-to-companion templates
- persistent pet ownership
- authoritative tame, summon, dismiss, mount, and dismount commands
- runtime companion presence in the known-set
- backend-owned mounted move speed
- classic HUD and scene projection without local success fallbacks

Status:

- concluida para o primeiro slice online autoritativo
- `mireling_strider` now ships as the canonical `pet_mount` template derived from a tameable `mireling`
- pet ownership persists in `character_pets` and rehydrates through `world/enter`, attach-time runtime load, and restart-safe PostgreSQL state
- summoned companions now appear as authoritative `pet` entities in `region_context`, `entity_appear`, `entity_disappear`, and read-model projection
- mount state now derives player move speed from backend pet state instead of client-owned presentation logic
- `ALT+C` now exposes authoritative `tame_target`, `summon_pet`, `dismiss_pet`, `mount_pet`, and `dismount_pet` shortcuts
- advanced pet combat, pet inventory, pet equipment, breeding, and large companion AI remain intentionally out of scope for this slice

### Fase L - Social core

Focus:

- authoritative party membership, invites, and leader rules
- runtime and `world/enter` party projection
- compact HUD party roster and invite affordances without fake local success
- authoritative social chat for `region`, `party`, and `whisper`
- stable notices for invite, join, decline, leave, kick, and leader transfer

Status:

- concluida para o primeiro slice de `party`
- the base party slice now follows the canonical minimum model: invite uses the current player target, invite TTL is 10 seconds, the party cap is 9, and the party is born or grows only on accept
- `parties`, `party_members`, and `party_invites` now persist the canonical party state with short-lived invite expiry; pending invite state stays ephemeral and cannot become durable fake success on the client
- `invite_party_member`, `accept_party_invite`, `decline_party_invite`, `leave_party`, and `kick_party_member` now run through the authoritative command lifecycle and durable dedup
- attach-time runtime load plus `world/enter.self_state.party` and `party_invites` now rehydrate roster truth and pending invites consistently with accept-time party birth
- the current player target is the only invite source in this slice; `/invite` and `/leave` are client-side affordances that normalize into authoritative gameplay commands instead of routing through `send_chat_message`
- the HUD now renders a compact party frame for roster, leave, and leader kick actions, while incoming invites use a dedicated classic modal centered above the hotbar with `Accept`, `Cancel`, and a countdown bar derived from authoritative `expires_at_ms`
- `ALT+C` now exposes authoritative `party_invite` and `party_leave` actions in addition to the existing social and companion shortcuts
- `send_chat_message` now validates `region`, `party`, `alliance`, and `whisper` on the backend, trims or bounds text, rate-limits burst spam, and rejects unknown channels with stable `chat.*` reason codes including `chat.alliance_required`
- `chat_message` now fans out only to same-region sessions, online or attached members of the sender's current party, all online or attached members of the sender's current alliance, or the named whisper target plus sender, without trusting client-side scope or delivery success
- `chat_messages` now persist minimum history in PostgreSQL or memory with actor, account, `alliance_id` when applicable, target, region, sanitized text, and command metadata for future auditability
- the bottom-left classic chat panel now renders safe escaped text plus a compact authoritative composer instead of fake local chat success
- dead actors remain allowed to use this first social chat slice; social chat is not blocked by combat death state in the current phase
- `local` remains reserved for a later distinct scope and is not exposed as a separate functional channel from `region` in the current slice
- only the current leader may invite or kick; self-invite, target already in party, duplicate invite, and full party reject with stable `party.*` reason codes
- disconnect of inviter or invitee cancels the pending invite; late accept after expiry or disconnect resolves as invite no longer valid
- the party does not functionally remain at one member: leave or kick that drops the roster to one dissolves the party, while leader leave with two or more remaining members transfers leadership deterministically to the oldest remaining member
- party reward sharing now exists in the minimum authoritative form: same-party, online/attached, same-region, alive members split kill XP deterministically, and party-owned loot allows pickup only for the eligible kill-time party subset
- party reward pickup keeps the existing persistent `character_items` path and the same first-valid-pickup-wins contention rule inside the eligible subset
- the first authoritative `clan` foundation is now online with `clans`, `clan_members`, and `clan_invites`, compact `self_state.clan` plus `clan_invites` hydration, and authoritative `create_clan`, `invite_clan_member`, `accept_clan_invite`, `decline_clan_invite`, `leave_clan`, `kick_clan_member`, and `dissolve_clan`
- clan creation now immediately persists the founder as leader plus first member; the clan remains valid at one member, the leader cannot use `leave_clan` in this phase, and dissolve stays explicit and leader-only with no auto-transfer or manual transfer
- clan invites now follow the same hardened minimum social semantics: current target only, 10-second TTL, one pending invite per invitee, one active outbound invite per clan or leader, and cancellation on inviter or invitee disconnect
- player targeting for social actions is now backend-owned through `select_target`; `invite_clan_member` has a strict empty payload and cannot accept a client-authored recipient
- clan accept now adds membership and consumes the invite atomically, while storage enforces one live outbound invite per clan and one live inbound invite per invitee
- all seven clan commands now have deterministic identical replay coverage, conflicting replay rejection, actor-delta command correlation, and regression coverage for invalid target, expiry, disconnect, reconnect hydration, kick, and dissolve
- a real two-character Docker Compose browser smoke now covers create, invite, accept, reconnect hydration, leave, decline, reaccept, kick, and dissolve without local success fallback
- `world/enter.self_state.clan` and `self_state.clan_invites` now rehydrate compact clan truth without fake local success; runtime deltas and `clan_notice` remain lifecycle feedback rather than state authority
- `ALT+N` now renders the minimum clan panel with `No Clan` plus `Create Clan` affordance when empty, compact roster when joined, leader-only `Invite`, `Kick`, and `Dissolve`, member-only `Leave`, and a dedicated non-draggable clan invite modal above the hotbar with countdown derived from authoritative `expires_at_ms`
- siege, clan war expansion, clan chat, clan warehouse, clan skills, academy, subunits, rich crest UX, privileges, `/invite Nome`, command channel, round-robin, master loot, dice or distribution UI, clan or alliance reward sharing, party finder, matchmaking, offline mail, manual leader transfer, and advanced moderation remain intentionally out of scope for this slice
- the first authoritative PvP/PK slice now reuses `basic_attack` and single-target `use_skill` for known players while preserving the existing mob/PvE path
- player selection remains target state only; player damage additionally validates live attachment, same region, fail-closed region PvP policy, alive state, party, clan, alliance, range, learned skill, cooldown, and MP
- player damage consumes CP before HP, applies death and simple respawn on the backend, and publishes no XP, loot, currency, or client-local success
- successful hostile hits start or refresh a durable absolute 30-second PvP deadline; reconnect/logical restart preserve it while valid and authoritative expiry clears it without client fallback, without dual timers or client-authored phase changes
- `stonecross_plaza` and compatibility region `dawn_plaza` now have a server-only spawn sanctuary policy (`x=-12..-4`, `z=-4..4`); unknown regions fail closed, without changing map, renderer, picking, geodata, bounds, spawn, checkpoints, or assets
- attacker and victim combat resources, exposure deadlines, PvP/PK counters, and detailed `pvp_combat_events` audit commit atomically before success; identical replay cannot reapply side effects or duplicate audit and conflicting replay remains explicit
- PostgreSQL now locks both combatant rows in deterministic character-id order and computes damage, MP, cooldown, death/classification, flags, counters, karma, attribution, repeated-pair signal, and audit from locked durable truth; the local mutex is only runtime coordination
- the memory adapter mirrors the same serialized mutation contract, while player combat skips the generic post-command flush that could otherwise overwrite a newer multi-instance state
- applied hits form a 30-second durable attribution ledger; a lethal event stores the final attacker as killer plus distinct recent assists, stopping at the victim's previous death boundary
- a repeated killer/victim kill inside 10 minutes stores `suspicious=true` and an inclusive `repeated_kill_count`; it is investigation-only and does not block damage or alter rewards
- player death clears offensive target, queued/automatic combat, queued loot approach, active movement, flag, and cooldown state; respawn restores a clean authoritative state
- token-gated `GET /internal/pvp/events` provides read-only attacker/victim/involved/killer/suspicious/action/result/time investigation filters
- frontend read-model and classic HUD project only authoritative flag, counters, karma, resources, and death; the compact self-state indicator is derived from authoritative `pvp_flagged` plus `pvp_flag_until_ms`, and a two-session Docker smoke covers basic attack, reconnect with active flag, and single-target skill outside the sanctuary
- PostgreSQL cross-instance fanout now delivers command-correlated remote-target notice, remote whisper, region chat, and party/clan lifecycle notices; claim is destination-safe, replay/client delivery is deduplicated, social state is rehydrated before notices, drift retries/dead-letters, and no remote damage or local target success is created
- durable recipient receipts now survive logical consumer restart, serialize competing consumers, and keep stale-owner/dead-letter paths free of visual success; party/clan command mutations now share the command/outbox transaction
- region chat now resolves active same-region ownership server-side, commits sanitized history + command outcome + one exact-owner event per remote recipient atomically, delivers local recipients only after commit, excludes other regions, and revalidates ownership/runtime region without automatic reroute
- regional player projection now publishes exact-recipient `upsert`/`despawn` events on attach, movement/state change, heartbeat, region change, and unregister; consumers keep projection-only known-set entities ordered by fence/version and expire stale visuals by TTL
- a dedicated bounded projection publisher now coalesces latest per source under pressure and exposes queue depth/capacity/coalesced/drop plus delivery-delay sum/count/max metrics
- the real two-backend Compose profile and Playwright scenario validate bidirectional projection/region chat, separate ownership, stop/restart, retry/dead-letter, receipts, TTL/despawn, stale tombstone rejection, reconnect fencing, and recovery under a movement burst
- remote social/projection delivery captures the recipient fence as well as session/instance/character, so the same durable session id under a newer fence cannot receive an event from the previous ownership epoch
- reconnect now hydrates backend-derived `next_command_seq`, preserving durable command dedup across the reused session instead of creating a false conflicting replay
- AoE/chain PvP, auto-approach/repeat against players, pet/summon or weighted attribution, anti-feed enforcement/correlation/alerting, richer named-zone/content volumes, karma recovery, economic penalties, wars, siege, olympiad, events, ranking, and rewards remain later slices

## Later

After the online foundation becomes secure, replay-safe, and observable, the roadmap can continue into:

- finer interest-anchor accuracy, broader social broadcast, and only then infrastructure expansion if measured load requires it
- protocol-level client consume acknowledgements only if the residual socket-accept/receipt-commit ambiguity becomes operationally unacceptable; reroute remains a separate explicit policy decision
- broader vendor and warehouse variants
- PvP/PK expansion beyond the hardened single-target slice: karma recovery, economic/death penalties, alerting, richer named-zone/content policy, and weighted/non-player attribution
- deeper anti-abuse enforcement, account/device correlation, and alerting on top of the current suspicious-event audit query
- instances and competitive endgame systems

## Current Online Slice Checklist

This checklist reflects the current state of the repository rather than the original pre-implementation plan.

### Implementado

- [x] The client cannot enter the online world without login, character entry, and `region_context`.
- [x] Gameplay commands use the official envelope.
- [x] The gameplay actor is derived from the authenticated session binding.
- [x] `ack`, `reject`, and `delta` are distinct.
- [x] Early rejects and semantic rejects exist.
- [x] Movement is authoritative on the server.
- [x] `known-set` gates entity-referencing commands.
- [x] `use_skill` is authoritative.
- [x] Loot pickup is authoritative and atomic, including server-owned approach movement when the drop is outside immediate range.
- [x] Inventory and equipment are durable.
- [x] Equipment-derived stats are authoritative.
- [x] Player HP and death state are reflected by authoritative snapshot and delta.
- [x] Quest state, NPC interactions, and consumable use remain backend-authoritative.

### Parcial

- [x] Password storage meets the minimum staging bar.
- [x] Access tokens are durable or otherwise restart-safe by documented design.
- [x] Command replay safety is durable across retries and restart boundaries.
- [x] Metrics and structured logging cover the main online paths.
- [x] E2E automation proves the browser flow over Docker.
- [x] Durable progression state covers more than inventory, equipment, and position.
- [x] Durable death or respawn checkpoints are explicit and restart-safe.
- [x] Other players appear, move, and disappear authoritatively in-region.

### Pendente

- [x] Movement is backed by server-owned terrain/geodata and pathfinding.
- [x] Static blockers cannot be crossed by client movement.
- [x] The server can generate alternate routes around common obstacles.
- [x] Browser movement reconciles pending path to authoritative route.
- [x] The local player starts moving immediately after terrain click with bounded prediction and smooth reconciliation.

## Fora

The following items remain explicitly out of scope for the current consolidation and hardening phases:

- microservices
- Redis in the gameplay truth path
- event sourcing
- reconnect beyond the current lease renewal, active-session reissue, expiry takeover, and fenced cleanup contract
- seamless-world expansion
- per-frame persistence of movement
- generic fallback between local and online authority
- party, clan, alliance, and broad social systems before presence is stable
- broad PvP, siege, olympiad, and other endgame systems before auth, dedup, and observability are ready
- client-side-only collision or pathfinding as gameplay truth
- client-supplied paths, waypoints, or collision results in gameplay commands
- client-side retry loops for loot pickup, combat approach, or other interaction legality

## Acceptance Criteria For This Backlog Update

- The document no longer claims that combat, loot, inventory, and equipment are still unimplemented.
- The document clearly separates shipped behavior from public-readiness gaps.
- The next execution priorities match `TRAE_SOLO_MASTER_PROMPT.md`.
- The terrain/geodata/pathfinding slice is explicit before expanding more gameplay systems on top of movement.
- The anti-scope remains explicit.
