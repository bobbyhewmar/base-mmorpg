# Online Readiness Checklist

## Purpose

Track what must be true before the current authoritative online slice can be considered ready for a first public or shared-staging environment.

This checklist is intentionally stricter than "feature exists". It is about whether the implemented slice is trustworthy, replay-safe, testable, and operable.

## Current Slice Already In Place

- [x] Register, login, character list, character creation, and `world/enter`
- [x] WebSocket attach and `region_context`
- [x] Authoritative movement and targeting
- [x] Authoritative skill use
- [x] Mob death and respawn
- [x] Player death and simple respawn
- [x] First authoritative single-target PvP/PK slice
- [x] Authoritative loot and pickup
- [x] Persistent inventory and equipment
- [x] Equipment-derived stats and authoritative player HP
- [x] Persistent quest state and authoritative NPC interaction services
- [x] Persistent pets, taming, summon or dismiss, and mount or dismount in a first authoritative slice
- [x] Classic HUD baseline for player status, target frame, chat, hotbar, inventory, and skill book

## Must Be True Before Public Exposure

### Documentation and Guidance

- [x] Local Docker documentation does not claim PostgreSQL support is still future work.
- [x] Public source-study docs do not expose a local absolute source-tree path.
- [x] The online backlog reflects what is already implemented versus what is still missing.
- [x] Core execution skills point back to `TRAE_SOLO_MASTER_PROMPT.md` when prioritizing autonomous work.

### Auth and Session Safety

- [x] Password hashing uses Argon2id or bcrypt instead of simple SHA-256.
- [x] Password algorithm versioning is explicit.
- [x] Access tokens or account sessions have real expiry handling.
- [x] Auth and attach endpoints have minimum rate limiting.
- [x] Sensitive values are not logged.
- [x] Every online character has one durable ownership row with `server_instance_id`, absolute lease, and monotonic fencing token.
- [x] Two backend instances racing the same attach credential produce one durable winner because successful acquisition rotates the credential atomically.
- [x] Authenticated reconnect of the same active session on its current instance drains serialized dispatch, advances the fence, and invalidates the old socket; another instance or session cannot replace an unexpired owner.
- [x] Gameplay commands renew and validate the exact owner tuple before dedup or `ack`; a superseded socket receives `session.stale_owner` without sequence or mutation.
- [x] Lease expiry permits a fenced replacement and closes a different previous session without letting the old socket release the new owner.
- [x] Release preserves an expired ownership tombstone so the next session continues the character's monotonic fencing sequence instead of resetting it.
- [x] Startup sanitation preserves unexpired ownership belonging to another server instance.
- [x] Release and unregister are conditional and idempotent, including double unregister.
- [x] Ownership acquire, renew, replace, conflict, expiry, stale reject, and release are observable without logging attach credentials.

### Command Integrity

- [x] Durable dedup exists for `session_id + command_seq`.
- [x] Identical replay returns the stored outcome without duplicating side effects.
- [x] Conflicting replay is rejected explicitly.
- [x] Dedup survives restart in PostgreSQL-backed mode.
- [x] Reconnect of a reused durable session receives backend-derived `next_command_seq` and does not restart the sequence namespace locally.

### Runtime Observability

- [x] HTTP requests expose latency and error counters.
- [x] Attach success and failure are observable.
- [x] Command `ack`, `reject`, and `delta` paths are observable.
- [x] Rejects are countable by `reason_code`.
- [x] Active sessions, active sockets, and region occupancy are visible.
- [x] Database errors are countable.

### Browser and Docker Confidence

- [x] An automated E2E flow covers register/login/create/enter/attach.
- [x] The E2E flow covers movement, targeting, skill use, loot, and equip/unequip.
- [x] The E2E flow runs against the local Docker topology or a clearly documented equivalent.

### Durable Gameplay State

- [x] HP and MP are durably restored by the current online progression rules.
- [x] XP and level persist through online combat progress.
- [x] Cooldown end timestamps persist when the rules require durability.
- [x] Restart or re-entry behavior for those fields is tested and documented.

### Economy Auditability

- [x] Vendor buy, vendor sell, and fixed exchange persist stable action rows without trusting client price or reward payloads.
- [x] Warehouse deposit and withdraw persist both `storage_transfer_records` and investigation-friendly `action_logs`.
- [x] Player trade persists auditable offer, accept, decline, send, and receive rows for the current slice.
- [x] Economic audit rows include actor and account identity, item identity, quantity, stable action type, server timestamp, and command metadata when available.
- [x] Internal read-only audit queries are token-gated, paginated, and separate from normal gameplay auth.
- [x] Audit queries cover filtering by character, item instance, action type, time window, warehouse history, and trade involvement.

### Multiplayer Reality

- [x] Other players appear and disappear in-region authoritatively.
- [x] Other players' movement is broadcast authoritatively.
- [x] Presence cleanup on disconnect is verified.
- [x] Durable ownership distinguishes a known player online on another instance from an offline or unknown player.
- [x] `select_target` and PvP reject remote-owned players with `presence.target_remote` and never create local fallback success; a previously authoritative social target may be revalidated for a remote party/clan invite.
- [x] PostgreSQL outbox provides monotonic ids, immutable idempotency keys, exact-instance claim leases, retry/dead-letter state, delivered-only retention, remote-target notice, remote whisper/region chat, party/clan lifecycle notices, and exact-recipient regional player projections.
- [x] Remote-target notice production is atomic with command finalization; identical replay cannot duplicate the event and conflicting replay remains rejected.
- [x] Remote whisper history, command outcome, and delivery intent commit atomically; identical replay does not duplicate history, event, or visual delivery.
- [x] Ownership drift uses explicit exact-session-and-fence retry/dead-letter with `social.recipient_offline` or `social.recipient_stale_owner`; reusing the same session id under a newer fence does not cross takeover, reroute, or fall back locally.
- [x] Two independent PostgreSQL stores cannot concurrently claim the same live delivery row.
- [x] Durable recipient receipts serialize concurrent consumers and suppress a consumed event after logical consumer restart.
- [x] Command-driven party/clan mutation, final command outcome, and remote outbox events commit or roll back as one PostgreSQL transaction; local fanout waits for commit.
- [x] Ownership drift releases unconsumed receipt reservations and preserves retry/dead-letter without local success or automatic reroute.
- [x] Same-region remote-owned players receive exact-recipient `presence.region_player_projection.v1` upsert/despawn events with durable receipts, source ownership revalidation, and fence/version ordering.
- [x] Regional projection redelivery and older events cannot duplicate, overwrite, or resurrect a newer entity; TTL removes stale visuals while retaining an ordering tombstone.
- [x] The browser reuses bounded other-player interpolation and does not set target or command success from projection appearance; backend selection/PvP remain `presence.target_remote`.
- [x] A separate Docker Compose `multi-backend` profile validates two real backend processes, distinct ownership, bidirectional projection and region chat without changing the default development services.
- [x] The multi-backend scenario stops/restarts one backend, verifies retry/dead-letter, receipt behavior, TTL/despawn, tombstone protection, stale-owner suppression, reconnect fencing, and recovery.
- [x] Projection publication has a bounded queue plus bounded latest-per-source coalescing, explicit pressure/drop telemetry, and delivery-delay sum/count/max metrics exercised by a reproducible movement burst.
- [ ] Party-chat broadcast, remote combat, remote entity authority, and mob/NPC/loot/pet replication remain later slices.

### Terrain and Pathfinding Reality

- [x] Region geodata is server-owned and versioned.
- [x] `move_intent` accepts destination point only, not client-supplied paths.
- [x] Static obstacles block movement on the backend.
- [x] The server can generate an alternate route around rocks, walls, and similar blockers.
- [x] Blocked or unreachable destinations produce stable movement reason codes.
- [x] Browser movement reconciles to the authoritative server route.
- [x] The local player starts moving immediately after a valid-looking terrain click without waiting for server pathfinding.
- [x] The client uses a bounded prediction leash and blends to the authoritative route under normal latency.
- [x] `clean_plain_1024_geo_v1` uses the canonical 1024x1024 playable bounds shared by client terrain, ground picking, spawn/checkpoint, and backend geodata.
- [x] Frontend picking tests keep near-edge clean-region clicks such as `x/z=+/-500` legal instead of clamping them into old-map bounds.
- [x] Backend movement tests accept traversal toward the intended north, south, east, and west region edges unless an explicit authored blocker applies.

### HUD Readiness

- [x] Inventory is closed by default and toggles through `ALT+V`.
- [x] Inventory uses compact icon-only `32x32px` slots with hover/focus details.
- [x] Inventory close control works as a real title-bar button.
- [x] Character-window family opens through `ALT+T`, `ALT+K`, `ALT+C`, `ALT+N`, and `ALT+U`.
- [x] Character-window top row switches panels instead of duplicating skill icons.
- [ ] Bottom-right quick access mini menu matches the classic reference with `32x32px` Status, Inventory, Map, and System buttons plus `ALT+T`, `ALT+V`, `ALT+M`, and `ALT+X` shortcuts.
- [ ] System menu opens as a classic square window and `Exit Game` shows an explicit `OK` or `Cancel` confirmation modal.
- [x] Skill book opens through `ALT+K`.
- [x] Skill book separates `Active` and `Passive` skills with compact icon grids.
- [x] Active skills can be dragged to hotbar slots for immediate local UI rebinding with the icon following the cursor during drag.
- [x] Inventory items can be dragged to hotbar slots as local shortcut bindings.
- [x] Equipable item shortcuts execute the equip flow when clicked.
- [x] Consumable item shortcuts execute through an authoritative `use_item` command.
- [x] `ALT+C` action shortcuts execute through authoritative action commands such as `basic_attack` and `pick_up_nearby`.
- [x] `ALT+C` now also exposes authoritative `party_invite` and `party_leave` shortcuts without introducing local party authority.
- [x] `pick_up_nearby` and direct drop clicks send one pickup command; server-owned approach movement collects loot after entering authoritative range.
- [x] Drag-to-hotbar rebinding for `skill`, `item`, and `action` persists through an authoritative backend command and reconnect.
- [x] Quest and NPC dialogs are projected from authoritative quest or interaction snapshots rather than local fallback truth.
- [x] Incoming party invites use a dedicated small modal above the hotbar with `Accept`, `Cancel`, and a visual countdown derived from authoritative `expires_at_ms`.
- [x] The invite modal may show an expired visual state locally, but final invite removal still comes only from authoritative backend state.

### Companion and Mount Reality

- [x] Tame success, ownership, summoned state, and mounted state are derived only on the backend.
- [x] `world/enter` and attach-time runtime hydration restore persisted pet ownership and mounted move speed safely after restart.
- [x] Summoned companions appear in the authoritative known-set as `pet` entities rather than local-only visuals.
- [x] Mounted move speed is derived from backend companion state, not from client payload or local toggles.
- [x] `ALT+C` companion actions dispatch only authoritative commands and do not fake local success for tame, summon, dismiss, mount, or dismount.
- [x] The current slice intentionally excludes advanced pet combat, pet inventory, pet equipment, breeding, and large AI behavior.

### Social Core Reality

- [x] Party membership, leader, leave, and kick are derived only on the backend.
- [x] Party invites use the actor's current player target rather than a client-authored name or freeform recipient payload.
- [x] Party invites persist with 10-second expiry, do not duplicate through replay or retry, and stay ephemeral until accept.
- [x] `world/enter` and attach-time runtime hydration restore authoritative `party` and `party_invites` snapshots.
- [x] Party roster or invite UI is a compact projection driven by authoritative deltas and notices, not by local success assumptions.
- [x] `/invite` and `/leave` are only client affordances that normalize into authoritative gameplay commands; party logic does not run through `send_chat_message`.
- [x] Self-invite, target-already-in-party, duplicate invite, and party-full cases reject with stable `party.*` semantics.
- [x] Disconnect of inviter or invitee cancels the pending invite; late accept after expiry or disconnect does not recreate stale party state.
- [x] The party cap is 9 and the party does not remain functional at one member; leave or kick that drops the roster to one dissolves the party.
- [x] Leader leave transfers leadership deterministically to the oldest remaining member when 2 or more members remain; manual leader transfer remains out of scope.
- [x] `send_chat_message` validates only the current functional channels (`region`, `party`, `whisper`), text bounds, whisper target lookup, party membership, and rate limits on the backend.
- [x] `chat_message` fans out only to same-region sessions, online party members, or the named whisper target plus sender.
- [x] A named whisper target online on another instance receives `social.chat_message.v1` through the outbox; text stays server-sanitized and bounded.
- [x] Region chat resolves active same-region ownership server-side, delivers local recipients only after commit, and creates one exact-owner outbox event plus durable receipt per remote recipient; other regions are excluded.
- [x] Party/clan invite, accept, decline, leave, kick, and dissolve lifecycle feedback reaches affected remote-owned members through idempotent outbox events.
- [x] Remote party/clan delivery rehydrates authoritative durable state into a delta before its notice; ack/notice alone cannot create membership or invite success.
- [x] Backend runtime and browser read-model suppress duplicate remote social `event_id` values, and sender echoes are deduplicated by authoritative chat `command_id`, without inventing local delivery success.
- [x] Receipt observability covers created, duplicate, and consumed without logging chat content; region-chat lifecycle additionally exposes produced, local delivered, remote enqueued/consumed, duplicate, stale-owner, and dead-letter.
- [x] Minimum chat history persists without trusting client-supplied scope, target identity, or message outcomes.
- [x] The HUD renders chat text in escaped form and does not execute HTML or JS from chat payloads.
- [x] Social chat remains available while the actor is dead; combat death state does not silently block the current social slice.
- [x] Party kill XP is shared only across the current authoritative eligible subset: same party, online and attached, same region, and alive at distribution time.
- [x] Party-owned loot can be picked up only by the eligible party subset, while out-of-party or ineligible actors receive a stable semantic reject.
- [x] `clans`, `clan_members`, and `clan_invites` persist the canonical minimum clan slice with replay-safe leader, membership, and short-lived invite state.
- [x] `create_clan`, `invite_clan_member`, `accept_clan_invite`, `decline_clan_invite`, `leave_clan`, `kick_clan_member`, and `dissolve_clan` execute only through the authoritative gameplay command lifecycle.
- [x] Clan creation immediately binds the founder as leader and first member, the clan remains valid with one member, and `dissolve_clan` is explicit plus leader-only.
- [x] Clan invites use the actor's current player target, expire after 10 seconds, reject self-invite or target-already-in-clan, and cancel when inviter or invitee disconnects.
- [x] Player selection used by social commands runs through authoritative `select_target`; selecting a player does not enable PvP/PK, and `invite_clan_member` rejects any client-authored recipient payload.
- [x] Clan invite acceptance adds membership and consumes the invite atomically, with storage-level uniqueness for one live outbound invite per clan and one live inbound invite per invitee.
- [x] Successful clan mutations correlate their actor-facing delta with `command_id + command_seq`; ack or an uncorrelated notice cannot produce projected clan success.
- [x] Identical replay is deterministic across all seven clan commands, conflicting replay rejects without mutation, and invalid target, expiry, and disconnect cases retain stable authoritative outcomes.
- [x] `world/enter` and attach-time runtime hydration restore authoritative `clan` and `clan_invites` snapshots without client-authored success.
- [x] Docker Compose browser smoke covers two characters through create, invite, accept, reconnect hydration, leave, decline, reaccept, kick, and dissolve.
- [x] `ALT+N` projects a compact clan panel with `Create Clan`, joined roster, leader-only `Invite` or `Kick` or `Dissolve`, member-only `Leave`, and a dedicated non-draggable clan invite modal above the hotbar.
- [x] Alliance, siege, clan war expansion, clan chat, clan warehouse, clan skills, academy or subunits, rich crest UX, complex privileges, and manual leader transfer remain intentionally out of scope for this slice.
- [x] `/invite Nome`, command channel, round-robin, master loot, dice or distribution UI, clan or alliance reward sharing, party finder, matchmaking, offline mail, manual leader transfer, and advanced moderation remain intentionally out of scope for this slice.

### PvP/PK Reality

- [x] `select_target` may select a player but never authorizes damage by itself.
- [x] `basic_attack` and learned single-target `use_skill` route player targets through a backend-owned PvP path while mob targets keep the existing PvE path.
- [x] PvP eligibility revalidates attached same-region known-set membership, fail-closed region policy, server-authored safe areas, life state, party, clan, range, cooldown, learned skill, and MP.
- [x] Player damage consumes CP before HP and player death, cooldown clear, and simple respawn remain backend-owned.
- [x] Successful hostile damage starts or refreshes an authoritative 30-second PvP deadline persisted as an absolute timestamp; reconnect/logical restart restore a still-active flag, and server-time expiry clears it durably before snapshot/delta projection.
- [x] Death-time classification increments durable `pvp_kills` for an exposed or karma-positive victim, or durable `pk_count` plus 100 karma for an unflagged karma-neutral victim.
- [x] Attacker and victim combat resources, exposure deadlines, PvP/PK consequences, and one detailed combat audit event commit atomically before success is published; lethal commits clear victim cooldown persistence.
- [x] PostgreSQL serializes each player-combat mutation with deterministic row locks on both combatants; the process-local mutex is no longer the multi-instance correctness boundary.
- [x] Damage, MP, attacker cooldown, death/classification, flags, counters, karma, lethal cooldown cleanup, attribution, anti-feed signal, and audit are computed from locked durable truth in one transaction.
- [x] The memory adapter mirrors the persistent mutation semantics in one critical section, including durable cooldown checks, attribution, and repeated-pair detection.
- [x] Identical replay does not reapply damage, death, PK, karma, MP, or cooldown; conflicting replay rejects explicitly.
- [x] A 30-second recent-hit ledger produces a primary killer plus distinct assists without crossing the victim's previous death boundary.
- [x] A second same-killer/same-victim kill inside 10 minutes is persisted as `suspicious` with `repeated_kill_count`, without blocking gameplay or changing rewards.
- [x] Backend and read-model tests cover player selection without damage, basic attack, skill, invalid/unknown/self target, target out of region, same party, same clan, restricted region, safe zone, out of range, dead target, disconnect, death cleanup, respawn, durable flag hydration/expiry, PK/PvP classification, audit, and replay.
- [x] Docker Compose browser smoke covers two attached users outside the spawn sanctuary through authoritative player selection, basic attack, active-flag reconnect hydration, single-target skill, flag projection, and victim resource projection.
- [x] `GET /internal/pvp/events` reuses the disabled-by-default internal audit token, supports pagination/time plus attacker/victim/involved/killer/suspicious/action/result filters, and exposes no mutation surface.
- [x] The HUD projects only backend-provided PvP flag, PvP kills, PK count, karma, CP, HP, and dead state.
- [x] AoE/chain PvP, player auto-approach/repeat, pets/summons, contribution weighting, anti-feed enforcement, clan/alliance wars, siege, olympiad, events, rankings, rewards, richer named-zone/content volumes, karma decay, and complex penalties remain explicitly deferred.

## Notes

- This checklist is not the full definition of the finished game.
- It is the minimum readiness gate for the current online authoritative slice to stop being only a local engineering milestone and become a credible public-ready base.
