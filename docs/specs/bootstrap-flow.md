# Bootstrap Flow

## Objective

Define the official bootstrap flow for `Fase 1.1` from a cold client to the first authoritative `RegionContext`.

This document freezes:

- the pre-game HTTP bootstrap
- the transition from account flow to character flow
- the transition from character entry to gameplay session creation
- the transition from gameplay session creation to `attach_session`
- the official point at which gameplay command flow may begin
- the main failure paths for each stage

This document does not redefine:

- `attach_session`
- `ack`
- `reject`
- `delta`
- `region_context`
- `revision`
- `region_revision`

Those remain governed by the already accepted authority and transport specifications.

## Decision

The official bootstrap flow for `Fase 1.1` is:

1. cold client starts in registration or login UI
2. player registers or logs in
3. client fetches authoritative character list
4. client fetches authoritative character-creation catalog
5. player selects an existing character or creates a new one
6. client requests world entry for one selected character
7. backend creates a gameplay session in `pending_attach`
8. client opens the gameplay WebSocket
9. client sends `attach_session`
10. backend validates the attach and binds the gameplay actor
11. backend sends authoritative `region_context`
12. gameplay command flow begins

The client must not enter the world directly from a cold start.

## Official Cold Client Flow

### Step 1: Cold Client Start

The cold client must open on:

- registration UI, or
- login UI

The cold client must not instantiate online world runtime before authenticated account flow and character entry succeed.

### Step 2: Registration Path

If the player chooses registration:

1. client sends `POST /v1/auth/register`
2. backend validates registration policy
3. backend persists the account if valid
4. backend returns the next required state

Valid success example:

```json
{
  "account_id": "acc_123",
  "registration_state": "created_pending_verification",
  "next_step": "login_or_verify"
}
```

After successful registration, the client either:

- proceeds to login, or
- shows pending-verification feedback when verification is required

Registration does not create a gameplay session.

### Step 3: Login Path

If the player logs in:

1. client sends `POST /v1/auth/login`
2. backend validates credentials and account state
3. backend returns authenticated account context and access token

Valid success example:

```json
{
  "account_id": "acc_123",
  "access_token": "access_abc",
  "expires_at_ms": 1730000000000,
  "account_state": "active"
}
```

Login does not create a gameplay session.

### Step 4: Character List and Catalog

After successful authentication, the client must fetch:

- `GET /v1/characters`
- `GET /v1/characters/catalog`

These calls serve different purposes:

- character list identifies owned characters
- character catalog identifies authoritative creation options

The client may render both screens from cached data after fetch, but the server remains authoritative.

### Step 5: Character Creation Path

If the player chooses to create a character:

1. client collects `race`, `base_class`, `sex`, `hair_style`, `hair_color`, `face`, and `name` from the authoritative catalog
2. client sends `POST /v1/characters`
3. backend validates all creation choices
4. backend persists the character if accepted
5. backend returns created character state and updated list view payload

Character creation is a client request, not a client-authoritative action.

Character creation does not create a gameplay session.

The selected appearance is not temporary preview data. It must persist on the character record, return in `GET /v1/characters`, flow into world bootstrap, and render as the exact player visual in the lobby and game world.

### Step 6: Character Entry Path

After the player selects one existing or newly created character:

1. client sends `POST /v1/world/enter`
2. backend validates:
   - authenticated account context
   - character ownership
   - entry eligibility
   - absence of conflicting active gameplay session
3. backend creates a gameplay session in `pending_attach`
4. backend returns:
   - `session_id`
   - `attach_token`
   - `attach_expires_at_ms`
   - `ws_url`

Valid success example:

```json
{
  "session_id": "sess_789",
  "character_id": "char_456",
  "attach_token": "attach_xyz",
  "attach_expires_at_ms": 1730000005000,
  "ws_url": "wss://game.example.com/v1/gameplay/ws"
}
```

World entry is the point at which the gameplay session is created.

### Step 7: `attach_session` Path

After `world/enter` succeeds:

1. client opens the gameplay WebSocket
2. client sends `attach_session`
3. backend validates:
   - session exists
   - session is attachable
   - `attach_token` is valid
   - session is not already attached
4. backend binds:
   - `socket -> session -> character`
5. backend establishes:
   - gameplay actor
   - expected initial `command_seq`
   - initial authoritative region view
6. backend sends `region_context`

The `attach_session` payload is:

```json
{
  "kind": "attach_session",
  "session_id": "sess_789",
  "attach_token": "attach_xyz"
}
```

## Official Start of Command Flow

Gameplay command flow starts only after the client receives authoritative `region_context`.

`region_context` is the effective success marker for attach.

Before `region_context` is received:

- gameplay command envelopes must not be sent
- movement and targeting may not begin online
- the client may show only loading or attaching feedback

## Full Sequence Example

1. Cold client opens on login or registration UI.
2. User registers or logs in.
3. Client becomes authenticated.
4. Client fetches characters and creation catalog.
5. User selects an existing character or creates one.
6. Client calls `POST /v1/world/enter`.
7. Backend creates gameplay session in `pending_attach`.
8. Backend returns `session_id`, `attach_token`, and `ws_url`.
9. Client opens WebSocket.
10. Client sends `attach_session`.
11. Backend validates attach and binds gameplay actor.
12. Backend sends `region_context`.
13. Client marks online gameplay bootstrap as complete.
14. Client starts sending gameplay commands beginning at the expected initial sequence.

## Main Failure Paths

### Registration Failures

Use at least:

- `auth.login_unavailable`
- `auth.password_policy_failed`
- `auth.rate_limited`

### Login Failures

Use at least:

- `auth.invalid_credentials`
- `auth.account_unverified`
- `auth.account_locked`
- `auth.rate_limited`

### Character Fetch Failures

Use at least:

- `auth.not_authenticated`

### Character Creation Failures

Use at least:

- `character.creation_limit_reached`
- `character.invalid_race`
- `character.invalid_base_class`
- `character.invalid_sex`
- `character.invalid_name`
- `character.name_unavailable`

### World Entry Failures

Use at least:

- `character.not_owned`
- `session.character_already_active`
- `session.entry_not_allowed`

### Attach Failures

Use at least:

- `session.not_found`
- `session.expired`
- `session.invalid_attach_token`
- `session.already_attached`
- `session.not_attachable`

## Out of Scope

The following remain out of scope for this document:

- combat bootstrap
- reconnect sophistication
- password recovery endpoint details
- verification endpoint details
- social or PvP bootstrap
- any client-side authority for entry legality

## Invariants

- The cold client never enters the world directly.
- Registration and login remain backend-authoritative.
- Character creation remains backend-authoritative.
- `world/enter` creates the gameplay session.
- `attach_session` binds the gameplay actor to the socket.
- `region_context` is the effective success marker for attach.
- Command flow starts only after `region_context`.

## Acceptance Criteria

- The cold client follows one mandatory path from auth to character entry before gameplay.
- No gameplay session exists before successful `world/enter`.
- No gameplay actor is bound to the socket before successful attach.
- No gameplay commands are accepted before `region_context`.
- All main failure categories are explicit and stable.
