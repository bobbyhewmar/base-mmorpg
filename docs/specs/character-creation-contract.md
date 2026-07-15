# Character Creation Contract

## Objective

Define the authoritative character-creation contract for the current MVP slice.

Character creation is backend-authoritative. The client renders options, previews visuals, and submits intent, but the backend owns legality, persistence, name reservation, and the final character shape.

## Current Canonical Scope

- Race: `Human` only.
- Base classes: `Fighter` and `Mage`.
- Sex: `Male` and `Female`.
- Appearance selectors: `hair_style`, `hair_color`, and `skin_type`.
- Body selection: no separate field. The selected `sex` chooses the only available body model for that sex.
- Removed fields: `face` and `body_type` are not part of the active creation contract.

Future body variants may be added only when the asset pack provides real body alternatives. Until then, exposing `Body Type` would be fake choice and is prohibited.

## Client Responsibilities

The client may:

- fetch `GET /v1/characters/catalog`
- preselect the first catalog-backed option for every required selector
- let the player type only a name and submit immediately
- preview the selected catalog-backed `sex`, `hair_style`, `hair_color`, and `skin_type`
- show backend errors for unavailable names or invalid selections

The client must not:

- invent races, classes, sex options, hairstyles, hair-color defaults, skin types, or body variants
- submit removed fields such as `face` or `body_type`
- render a `Body Type` selector while each sex has exactly one body
- treat a preview as persisted state before backend confirmation
- silently substitute a missing canonical asset with a different character asset

## Catalog Endpoint

`GET /v1/characters/catalog`

Success example:

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

Minimum rules:

- Catalog values are read-only client inputs.
- The backend must still validate every submitted value at creation time.
- The UI may disable submit only while the name is empty or required catalog-backed values are missing.
- Duplicate or reserved names are decided by `POST /v1/characters`, not by client-only logic.

## Create Character Endpoint

`POST /v1/characters`

Request example:

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

Success response example:

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

## Minimum Reason Codes

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
- `character.invalid_skin_type`
- `character.invalid_name`
- `character.name_too_short`
- `character.name_too_long`
- `character.name_contains_invalid_characters`
- `character.name_reserved`
- `character.name_profanity_blocked`
- `character.name_unavailable`

## Persistence And Runtime Projection

The persisted character summary must include:

- `race`
- `base_class`
- `sex`
- `hair_style`
- `hair_color`
- `skin_type`
- `name`

The same values must be projected into:

- character list
- character lobby preview
- `world/enter` bootstrap
- gameplay presence for self and other players
- Three.js character renderer

The lobby and world renderer must not quietly substitute local prototype defaults. If the user selected `Human`, `Female`, `Mage`, `hair_style = 2`, `hair_color = "#8f5fd3"`, and `skin_type = 1`, every subsequent screen must use that exact persisted shape.

## Asset Mapping

- `sex = Male` maps to the male Universal Base Characters body.
- `sex = Female` maps to the female Universal Base Characters body.
- `hair_style` maps to sex-compatible hair assets from the Universal Base Characters pack.
- `hair_color` is a canonical `#RRGGBB` string and tints only hair materials/meshes; it must not affect skin/body materials.
- `skin_type` maps to available sex-compatible skin textures.
- Equipment visuals are separate future content and must come from authoritative equipped items, not from base class selection.

## Testing Requirements

- Catalog response contains `hair_styles`, `hair_color_default`, and `skin_types`, and does not contain `faces` or `body_types`.
- Create request accepts `hair_color` as `#RRGGBB`, accepts `skin_type`, and rejects invalid values.
- Create request does not require any body option beyond `sex`.
- Character list and world presence include `hair_color` and `skin_type`.
- Lobby and world preview render from persisted values.
