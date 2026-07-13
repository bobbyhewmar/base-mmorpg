# Character Creation Contract

## Objective

Define the authoritative character-creation contract for `Fase 1.1`.

This document freezes:

- the authoritative creation catalog
- the contract for `GET /v1/characters/catalog`
- the contract for `POST /v1/characters`
- validation of `race`
- validation of `base_class`
- validation of `sex`
- validation of canonical appearance options
- validation of `name`
- success responses
- minimum reason codes

This document does not permit any client-side authority for creation legality.

## Decision

Character creation in `Fase 1.1` is backend-authoritative.

The client may:

- fetch the authoritative creation catalog
- render character-creation UI
- submit character-creation intent

The client must not:

- decide whether a race is enabled
- decide whether a base class is legal for a race
- decide whether a sex option is legal
- decide whether a hairstyle, hair color, or face option is legal
- replace missing creation appearance with a local default or fallback
- decide whether a name is valid or available
- persist a character as if creation succeeded without backend confirmation

## Authoritative Creation Catalog

The backend must expose a catalog endpoint:

- `GET /v1/characters/catalog`

The catalog is authoritative for UI rendering inputs.

The catalog is not a substitute for validation at creation time.

## Contract: `GET /v1/characters/catalog`

### Purpose

Return the server-owned creation options for `race`, `base_class`, `sex`, and canonical appearance fields.

### Success Example

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
        "hair_colors": [0, 1, 2],
        "faces": [0, 1, 2]
      }
    },
    {
      "race": "Elf",
      "enabled": true,
      "base_classes": ["Fighter", "Mage"],
      "sex_options": ["Male", "Female"],
      "appearance_options": {
        "hair_styles": [0, 1, 2],
        "hair_colors": [0, 1, 2],
        "faces": [0, 1, 2]
      }
    },
    {
      "race": "Dark Elf",
      "enabled": true,
      "base_classes": ["Fighter", "Mage"],
      "sex_options": ["Male", "Female"],
      "appearance_options": {
        "hair_styles": [0, 1, 2],
        "hair_colors": [0, 1, 2],
        "faces": [0, 1, 2]
      }
    },
    {
      "race": "Orc",
      "enabled": true,
      "base_classes": ["Fighter", "Mage"],
      "sex_options": ["Male", "Female"],
      "appearance_options": {
        "hair_styles": [0, 1, 2],
        "hair_colors": [0, 1, 2],
        "faces": [0, 1, 2]
      }
    },
    {
      "race": "Dwarf",
      "enabled": true,
      "base_classes": ["Fighter"],
      "sex_options": ["Male", "Female"],
      "appearance_options": {
        "hair_styles": [0, 1, 2],
        "hair_colors": [0, 1, 2],
        "faces": [0, 1, 2]
      }
    }
  ]
}
```

### Minimum Rules

- The client must treat the catalog as read-only.
- The client may cache the catalog only for UX.
- The server must still validate every submitted creation request against current authoritative rules.
- The client must not render an enabled appearance selector from hardcoded local options when the catalog does not provide those options.
- The creation preview may render only from selected catalog-backed options; no temporary visual fallback should be promoted to gameplay state.

### Minimum Reason Codes

- `auth.not_authenticated`

## Contract: `POST /v1/characters`

### Request Example

```json
{
  "race": "Human",
  "base_class": "Fighter",
  "sex": "Female",
  "hair_style": 1,
  "hair_color": 2,
  "face": 1,
  "name": "Arden"
}
```

### Success Response Example

```json
{
  "character": {
    "character_id": "char_456",
    "name": "Arden",
    "race": "Human",
    "base_class": "Fighter",
    "sex": "Female",
    "hair_style": 1,
    "hair_color": 2,
    "face": 1,
    "level": 1,
    "last_region_id": "dawn_plaza",
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
      "hair_color": 2,
      "face": 1,
      "level": 1,
      "last_region_id": "dawn_plaza",
      "is_enterable": true
    }
  ]
}
```

### Required Authentication

`POST /v1/characters` requires authenticated account context.

### Minimum Reason Codes

- `auth.not_authenticated`
- `character.creation_limit_reached`
- `character.invalid_race`
- `character.race_disabled`
- `character.invalid_base_class`
- `character.base_class_not_allowed_for_race`
- `character.invalid_sex`
- `character.sex_not_allowed_for_template`
- `character.invalid_hair_style`
- `character.invalid_hair_color`
- `character.invalid_face`
- `character.invalid_name`
- `character.name_too_short`
- `character.name_too_long`
- `character.name_contains_invalid_characters`
- `character.name_reserved`
- `character.name_profanity_blocked`
- `character.name_unavailable`

## Validation: `race`

The backend validates:

- the `race` field is present
- the submitted race exists in the authoritative race set
- the submitted race is enabled for the current environment

The initial accepted race identifiers are:

- `Human`
- `Elf`
- `Dark Elf`
- `Orc`
- `Dwarf`

### Rejections

- `character.invalid_race`
- `character.race_disabled`

## Validation: `base_class`

The backend validates:

- the `base_class` field is present
- the submitted base class exists in the initial allowed set
- the submitted base class is legal for the selected `race`

The initial base-class identifiers are:

- `Fighter`
- `Mage`

### Rejections

- `character.invalid_base_class`
- `character.base_class_not_allowed_for_race`

## Validation: `sex`

The backend validates:

- the `sex` field is present
- the submitted sex exists in the allowed representation set
- the submitted sex is legal for the chosen creation template

This phase assumes a project-owned supported sex set such as:

- `Male`
- `Female`

The backend remains authoritative if the set changes later.

### Rejections

- `character.invalid_sex`
- `character.sex_not_allowed_for_template`

## Validation: canonical appearance

The backend validates:

- `hair_style` is present and exists in the selected race template's `appearance_options.hair_styles`
- `hair_color` is present and exists in the selected race template's `appearance_options.hair_colors`
- `face` is present and exists in the selected race template's `appearance_options.faces`

Accepted values in the first implementation are numeric option identifiers owned by the backend catalog:

- `0`
- `1`
- `2`

These identifiers are not disposable preview flags. They are persisted on the character record, returned by `GET /v1/characters`, injected into gameplay presence, and rendered by the lobby/world 3D character visuals.

### Rejections

- `character.invalid_hair_style`
- `character.invalid_hair_color`
- `character.invalid_face`

## Validation: `name`

The backend validates:

- presence
- normalization
- minimum length
- maximum length
- allowed character set
- reserved-word policy
- profanity policy
- uniqueness

The client may perform advisory validation for UX only.

The client must not interpret advisory validation as authoritative success.

### Rejections

- `character.invalid_name`
- `character.name_too_short`
- `character.name_too_long`
- `character.name_contains_invalid_characters`
- `character.name_reserved`
- `character.name_profanity_blocked`
- `character.name_unavailable`

## Common Failure Shape

Character-creation failure responses should use a stable minimal shape:

```json
{
  "reason_code": "character.name_unavailable",
  "message": "Character name is unavailable."
}
```

## Anti-Examples

### Invalid: client invents race legality

If the client shows `Dwarf -> Mage` as selectable due to stale local UI and submits it anyway, the backend must still reject it if the authoritative catalog disallows it.

### Invalid: client trusts local name check

If the client previously marked `Arden` as available but another request reserves it first, the backend must reject creation with `character.name_unavailable`.

### Invalid: local-only creation success

The client must not create a temporary authoritative character record locally and proceed to world entry without backend creation success.

### Invalid: appearance fallback after creation

If the user selected `Orc`, `Female`, `Mystic`, `hair_style = 2`, `hair_color = 1`, and `face = 0`, every subsequent screen must use that exact persisted shape. The lobby and world renderer must not quietly substitute local prototype defaults.

## Invariants

- Character creation is server-authoritative.
- The catalog is authoritative for rendering inputs, not for skipping validation.
- `race`, `base_class`, `sex`, `hair_style`, `hair_color`, `face`, and `name` are all validated on the backend.
- Appearance choices are canonical persisted state, not temporary preview state.
- `GET /v1/characters`, region presence, and the 3D renderers must preserve the selected appearance exactly.
- Missing or invalid canonical appearance data should fail loudly at the boundary instead of being replaced with a client fallback.
- Name normalization and uniqueness are backend-only concerns.
- The client never decides that creation succeeded.

## Out of Scope

The following remain out of scope for this document:

- later class transfers
- subclass systems
- stat allocation customization beyond the accepted template choices
- cosmetic customization beyond the first canonical `hair_style`, `hair_color`, and `face` option sets
- economic or item-related creation bonuses

## Acceptance Criteria

- The creation catalog is returned by the backend.
- Character creation request shape is stable and explicit.
- Backend validation covers `race`, `base_class`, `sex`, `hair_style`, `hair_color`, `face`, and `name`.
- Name conflicts are rejected explicitly.
- Client-side advisory validation never overrides backend validation.
- The selected appearance persists in PostgreSQL and is returned unchanged when the character list is reloaded.
- World bootstrap and remote player presence include the persisted appearance fields.
