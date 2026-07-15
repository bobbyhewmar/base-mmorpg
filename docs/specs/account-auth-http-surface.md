# Account Auth HTTP Surface

## Objective

Define the minimum HTTP surface for account access and character entry in `Fase 1.1`.

This document freezes:

- the minimum HTTP endpoints required by the accepted bootstrap flow
- the authentication model per endpoint
- success and failure shapes at the HTTP layer
- supported account states visible to the client in this phase
- verification and recovery hooks without specifying full subsystems

This document does not redefine gameplay WebSocket attach or outbound gameplay message flow.

## Decision

The minimum HTTP surface for `Fase 1.1` is:

- `POST /v1/auth/register`
- `POST /v1/auth/login`
- `GET /v1/characters`
- `GET /v1/characters/catalog`
- `POST /v1/characters`
- `POST /v1/world/enter`

All endpoints must use HTTPS in online environments.

## Authentication Model

| Endpoint | Auth Required | Purpose |
| --- | --- | --- |
| `POST /v1/auth/register` | No | Create account and return next required step |
| `POST /v1/auth/login` | No | Authenticate account and issue access token |
| `GET /v1/characters` | Yes | Return authoritative list of account-owned characters |
| `GET /v1/characters/catalog` | Yes | Return authoritative character-creation catalog |
| `POST /v1/characters` | Yes | Create a character |
| `POST /v1/world/enter` | Yes | Validate entry and create gameplay session in `pending_attach` |

Authenticated endpoints must reject missing or invalid account context with:

- `auth.not_authenticated`

Authentication credentials are persisted with explicit password-algorithm versioning.

- New credentials use `bcrypt_v1`.
- Legacy `sha256` credentials may still exist only as a migration boundary and must be upgraded on successful login.

Access tokens are opaque backend-issued account-session records with explicit expiry.

## Supported Account States

The client-visible account states in this phase are:

- `active`
- `pending_verification`
- `locked`

These states are sufficient for the bootstrap flow.

This document does not define a full account-state machine beyond what is necessary for `Fase 1.1`.

## Endpoint Contracts

### `POST /v1/auth/register`

#### Purpose

Create an account and return the next required pre-game step.

#### Request Example

```json
{
  "login": "arden@example.com",
  "password": "Secret123!",
  "display_name": "Arden"
}
```

#### Success Response Example

```json
{
  "account_id": "acc_123",
  "registration_state": "created_pending_verification",
  "next_step": "login_or_verify"
}
```

Alternative success when verification is not blocking:

```json
{
  "account_id": "acc_123",
  "registration_state": "created_active",
  "next_step": "login"
}
```

#### Minimum Reason Codes

- `auth.login_unavailable`
- `auth.password_policy_failed`
- `auth.rate_limited`

### `POST /v1/auth/login`

#### Purpose

Authenticate account credentials and issue access token for subsequent HTTP calls.

The issued `access_token` is an opaque backend session token, not a client-derived identity artifact.

#### Request Example

```json
{
  "login": "arden@example.com",
  "password": "Secret123!"
}
```

#### Success Response Example

```json
{
  "account_id": "acc_123",
  "access_token": "access_abc",
  "expires_at_ms": 1730000000000,
  "account_state": "active"
}
```

#### Minimum Reason Codes

- `auth.invalid_credentials`
- `auth.account_unverified`
- `auth.account_locked`
- `auth.rate_limited`

### `GET /v1/characters`

#### Purpose

Return only the characters owned by the authenticated account.

#### Success Response Example

```json
{
  "characters": [
    {
      "character_id": "char_456",
      "name": "Arden",
      "race": "Human",
      "base_class": "Fighter",
      "sex": "Female",
      "hair_style": 1,
      "hair_color": "#8f5fd3",
      "skin_type": 2,
      "level": 1,
      "last_region_id": "stonecross_plaza",
      "is_enterable": true
    }
  ]
}
```

#### Minimum Reason Codes

- `auth.not_authenticated`

### `GET /v1/characters/catalog`

#### Purpose

Return the authoritative creation catalog used by the client to render creation UI.

#### Success Response Example

```json
{
  "races": [
    {
      "race": "Human",
      "enabled": true,
      "base_classes": ["Fighter", "Mage"],
      "sex_options": ["Male", "Female"],
      "appearance_options": {
        "hair_styles": [0, 1, 2],
        "hair_color_default": "#6b4e37",
        "skin_types": [0, 1, 2]
      }
    }
  ]
}
```


#### Minimum Reason Codes

- `auth.not_authenticated`

### `POST /v1/characters`

#### Purpose

Create a character from the authoritative creation contract.

#### Request Example

```json
{
  "race": "Human",
  "base_class": "Fighter",
  "sex": "Female",
  "hair_style": 1,
  "hair_color": "#8f5fd3",
  "skin_type": 2,
  "name": "Arden"
}
```

#### Success Response Example

```json
{
  "character": {
    "character_id": "char_456",
    "name": "Arden",
    "race": "Human",
    "base_class": "Fighter",
    "sex": "Female",
    "hair_style": 1,
    "hair_color": "#8f5fd3",
    "skin_type": 2,
    "level": 1,
    "last_region_id": "stonecross_plaza",
    "is_enterable": true
  },
  "characters": [
    {
      "character_id": "char_456",
      "name": "Arden",
      "race": "Human",
      "base_class": "Fighter",
      "sex": "Female",
      "hair_style": 1,
      "hair_color": "#8f5fd3",
      "skin_type": 2,
      "level": 1,
      "last_region_id": "stonecross_plaza",
      "is_enterable": true
    }
  ]
}
```

#### Minimum Reason Codes

- `auth.not_authenticated`
- `character.creation_limit_reached`
- `character.invalid_race`
- `character.invalid_base_class`
- `character.invalid_sex`
- `character.invalid_hair_style`
- `character.invalid_hair_color`
- `character.invalid_skin_type`
- `character.invalid_name`
- `character.name_unavailable`

### `POST /v1/world/enter`

#### Purpose

Validate character entry and create gameplay session in `pending_attach`.

#### Request Example

```json
{
  "character_id": "char_456"
}
```

#### Success Response Example

```json
{
  "session_id": "sess_789",
  "character_id": "char_456",
  "attach_token": "attach_xyz",
  "attach_expires_at_ms": 1730000005000,
  "ws_url": "wss://game.example.com/v1/gameplay/ws"
}
```

#### Minimum Reason Codes

- `auth.not_authenticated`
- `character.not_owned`
- `session.character_already_active`
- `session.entry_not_allowed`

## Verification and Recovery Hooks

`Fase 1.1` must support verification and recovery as explicit account-flow states, but this document does not define their full endpoint subsystems.

The bootstrap flow must still support:

- `auth.account_unverified`
- pending-verification UI
- password-recovery UI entry point

These remain hooks in the flow, not fully specified endpoint surfaces in this document.

## Common HTTP Failure Shape

HTTP failures should use a stable minimal response shape:

```json
{
  "reason_code": "auth.invalid_credentials",
  "message": "Invalid login or password."
}
```

This document does not require a full global error envelope beyond stable `reason_code` and `message`.

## Out of Scope

The following remain out of scope for this document:

- verification endpoint details
- recovery endpoint details
- gameplay WebSocket transport
- combat endpoints
- admin or live-ops HTTP surface

## Invariants

- Registration and login are backend-authoritative.
- Password legality and password verification stay fully on the backend.
- Access-token validity and expiry stay fully on the backend.
- Authenticated account context is required for character list, catalog, creation, and world entry.
- Character creation is not valid without an authenticated account.
- World entry is not valid without authenticated ownership of the character.
- HTTP surface ends at gameplay session creation; WebSocket attach starts after `world/enter`.

## Acceptance Criteria

- `Fase 1.1` exposes all minimum HTTP endpoints required by the accepted bootstrap flow.
- Character list and creation catalog are both fetched from the backend.
- Character creation is driven by server-owned options and server-owned validation.
- `world/enter` returns the data required for `attach_session`.
- Verification and recovery remain represented as explicit flow hooks without forcing a full subsystem redesign here.
