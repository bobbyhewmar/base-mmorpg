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

### Command Integrity

- [x] Durable dedup exists for `session_id + command_seq`.
- [x] Identical replay returns the stored outcome without duplicating side effects.
- [x] Conflicting replay is rejected explicitly.
- [x] Dedup survives restart in PostgreSQL-backed mode.

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
- [x] Minimum chat history persists without trusting client-supplied scope, target identity, or message outcomes.
- [x] The HUD renders chat text in escaped form and does not execute HTML or JS from chat payloads.
- [x] Social chat remains available while the actor is dead; combat death state does not silently block the current social slice.
- [x] Party kill XP is shared only across the current authoritative eligible subset: same party, online and attached, same region, and alive at distribution time.
- [x] Party-owned loot can be picked up only by the eligible party subset, while out-of-party or ineligible actors receive a stable semantic reject.
- [x] `clans`, `clan_members`, and `clan_invites` persist the canonical minimum clan slice with replay-safe leader, membership, and short-lived invite state.
- [x] `create_clan`, `invite_clan_member`, `accept_clan_invite`, `decline_clan_invite`, `leave_clan`, `kick_clan_member`, and `dissolve_clan` execute only through the authoritative gameplay command lifecycle.
- [x] Clan creation immediately binds the founder as leader and first member, the clan remains valid with one member, and `dissolve_clan` is explicit plus leader-only.
- [x] Clan invites use the actor's current player target, expire after 10 seconds, reject self-invite or target-already-in-clan, and cancel when inviter or invitee disconnects.
- [x] `world/enter` and attach-time runtime hydration restore authoritative `clan` and `clan_invites` snapshots without client-authored success.
- [x] `ALT+N` projects a compact clan panel with `Create Clan`, joined roster, leader-only `Invite` or `Kick` or `Dissolve`, member-only `Leave`, and a dedicated non-draggable clan invite modal above the hotbar.
- [x] Alliance, siege, clan war expansion, clan chat, clan warehouse, clan skills, academy or subunits, rich crest UX, complex privileges, and manual leader transfer remain intentionally out of scope for this slice.
- [x] `/invite Nome`, command channel, round-robin, master loot, dice or distribution UI, clan or alliance reward sharing, party finder, matchmaking, offline mail, manual leader transfer, and advanced moderation remain intentionally out of scope for this slice.

## Notes

- This checklist is not the full definition of the finished game.
- It is the minimum readiness gate for the current online authoritative slice to stop being only a local engineering milestone and become a credible public-ready base.
