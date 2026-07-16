# Account Auth and Character Entry

## Objective

Define the authoritative pre-gameplay flow for account access, character creation, character selection, and world entry.

This document freezes:

- the mandatory login or registration flow before gameplay
- the authoritative boundaries for account and character-entry logic
- the initial character-creation model by race, base class, sex, hairstyle, skin type, and name
- the security posture between client and backend during pre-gameplay flows
- the rule that the client may request creation choices but never decides their legality

## Decision

The first online slice must no longer drop the player directly into the world from a cold client.

The mandatory flow is:

1. register or log in
2. complete account verification requirements when needed
3. fetch the authoritative character list and creation catalog
4. select an existing character or create a new one
5. request character entry
6. attach the resulting gameplay session to the online client flow
7. treat authoritative `region_context` as the effective attach success marker before gameplay commands begin

The backend is authoritative for every step in this flow.

## Authority Boundaries

### Client Responsibilities

The client may:

- render login, registration, verification, recovery, character-list, and character-creation UI
- collect credentials and character-creation choices
- display server-provided validation feedback
- request world entry for a selected character

The client must not:

- authenticate itself by local decision
- mark an account as verified
- decide which races are enabled
- decide which base classes are legal for a race
- decide whether a sex choice is valid for the current creation template
- decide whether a hairstyle or skin type choice is valid for the current creation template
- replace missing persisted appearance with local prototype defaults
- decide whether a character name is valid or unique
- create a gameplay session by itself
- enter the world without backend approval

### Backend Responsibilities

The backend is authoritative for:

- account registration legality
- credential validation
- account verification and recovery rules
- rate limits and abuse controls for auth flows
- character-list ownership
- race, base-class, sex, hairstyle, skin type, and name legality
- character-name normalization, reservation, and uniqueness
- world-entry permission
- gameplay-session issuance

## Security Rules

- Use HTTPS for registration, login, verification, recovery, character creation, and world-entry endpoints.
- Use WSS for the gameplay session after world entry is accepted.
- Treat all client-submitted credentials, names, and creation choices as untrusted input.
- Keep account tokens, entry tokens, and gameplay-session attach tokens server-issued and server-validated.
- Fail closed on invalid credentials, invalid session state, conflicting active sessions, and invalid creation requests.
- Record structured rejection reasons for suspicious login, attach, or character-creation attempts.

## Mandatory Entry Screens

The browser client must expose these pre-gameplay screens:

- login
- registration
- account verification or pending-verification feedback
- password recovery
- character list
- character creation
- explicit session-expired or forced-logout feedback

## Initial Character-Creation Model

### Race

The current online slice exposes a deliberately narrow server-owned playable race catalog while the visual identity is stabilized.

The initial race set is:

- `Human`

The backend owns whether each race is enabled.

### Base Class

The initial creation choice must be grouped by base class:

- `Fighter`
- `Mage`

The backend owns which base-class choices are legal for the selected race.

The client may show only the options returned by the backend catalog, but the backend must still validate the submitted combination.

### Sex

The player may choose the character sex.

The backend owns whether a requested sex is valid for the chosen race and base-class template.

### Appearance

The player may choose canonical visible appearance options:

- `hair_style`
- `hair_color`
- `skin_type`

The HTTP field for skin type is `skin_type`. The HTTP field for hair color is `hair_color`, validated as canonical `#RRGGBB`. There is no active `face` or `body_type` creation field. `sex` selects the only available body model for that sex until real body variants exist.

The backend catalog owns the legal option identifiers for the selected race/template. These options are persisted character state, returned by the character list, propagated through gameplay presence, and rendered by the lobby/world character visuals.

The client may preview only selected catalog-backed appearance values. It must not invent fallback appearance if the backend does not provide or accept the selected option.

### Name

The player may propose a character name.

The backend is authoritative for:

- normalization
- allowed characters
- reserved-word rejection
- profanity filtering
- uniqueness
- final persistence

The client may perform advisory validation for UX only, but this must never replace backend validation.

## Character-Creation Catalog

The client must not hardcode character-creation legality as truth.

The backend should provide an authoritative creation catalog that expresses at least:

- enabled races
- enabled base classes for each race
- supported sex options
- supported `hair_style`, `hair_color_default`, and skin type options for each race/template
- optional presentation metadata for UI rendering

The client may cache this catalog for rendering, but the cache is not authority.

The creation UI should preselect the first enabled catalog-backed option for every required selector:

- race
- base class
- sex
- `hair_style`
- `hair_color`
- `skin_type`

This lets a player type only the character name and submit immediately. The preselection is a UI convenience from server-owned data, not a client-side legality decision.

## Entry and Creation Flow

### Registration

The client submits registration intent.

The backend validates:

- account-creation policy
- email or login uniqueness
- password policy
- rate limits
- verification requirements

On success, the backend persists the account and returns the next required step.

### Login

The client submits credentials.

The backend validates:

- credentials
- account status
- verification status
- lockout or abuse state

On success, the backend returns authenticated account context and the authoritative character list.

### Character List

The client fetches the character list from the backend after successful authentication.

The client also fetches the authoritative character-creation catalog needed to render creation choices.

The backend returns only characters owned by that account.

### Character Creation

The client submits:

- race
- base class
- sex
- proposed name

The backend validates:

- account permission to create a character
- race availability
- race and base-class compatibility
- sex compatibility
- name normalization and uniqueness

On success, the backend persists the character and returns updated character-list state.

### Character Entry

The client requests world entry for one selected character.

The backend validates:

- character ownership
- account authorization
- current session state
- duplicate active-session conflicts
- durable character ownership lease and current server instance
- entry preconditions

On success, the backend creates or reuses the appropriate gameplay-session record and returns the information required to continue into the online gameplay attach flow.

An authenticated reconnect may receive the existing session and current rolling attach credential while its durable ownership remains active. WebSocket attach still must acquire the character fence. On the current server instance, that same session first drains serialized command dispatch, then atomically rotates the credential and advances the fence, so exactly one reconnect wins and the previous socket becomes stale. Another server instance or a different gameplay session cannot replace an unexpired owner; after release or expiry it may acquire a higher fence and conditionally close the prior session.

The online gameplay attach flow is not complete until authoritative `region_context` is received.

## Rejection Examples

Stable rejection categories for this flow should cover at least:

- `auth.invalid_credentials`
- `auth.account_unverified`
- `auth.account_locked`
- `auth.rate_limited`
- `character.not_owned`
- `character.creation_limit_reached`
- `character.invalid_race`
- `character.invalid_base_class`
- `character.invalid_sex`
- `character.invalid_name`
- `character.name_unavailable`
- `session.character_already_active`
- `session.ownership_conflict`
- `session.stale_owner`
- `session.entry_not_allowed`

## Anti-Examples

- Directly entering the world from the client without account login and character entry.
- Letting the client decide that a name is available without backend confirmation.
- Letting the client decide which races or base classes exist.
- Using browser-local flags as authority for account verification.
- Accepting a character-creation request without backend normalization and uniqueness checks.

## Invariants

- The client never enters the world directly from a cold start.
- The backend is authoritative for account identity.
- The backend is authoritative for character ownership and entry permission.
- Race, base class, sex, hairstyle, skin type, and name legality are backend-only concerns.
- Character creation is a request from the client, not an authoritative client action.
- The same account may only enter the world through an accepted backend path.
- Gameplay authority belongs only to the exact durable `session + character + server instance + fencing token` tuple.

## Acceptance Criteria

- A cold client lands on login or registration, not directly in the game world.
- Successful login returns the authoritative character list for the account.
- Character creation is split by race, base class, sex, hairstyle, skin type, and name.
- The backend validates all character-creation choices before persistence.
- The backend rejects duplicate or invalid names explicitly.
- World entry requires authenticated account context and valid character ownership.
- Gameplay command flow begins only after successful attach and authoritative `region_context`.

## Related Documents

- `docs/specs/account-auth-http-surface.md`
- `docs/specs/character-creation-contract.md`
- `docs/specs/bootstrap-flow.md`
