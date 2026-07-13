# Command Envelope

## Objective

Define the official gameplay command envelope for the first authoritative online slice.

This document freezes:

- the envelope shape
- field semantics
- field origin
- field authority
- idempotency rules
- prohibited fields

This document applies to gameplay commands sent through the online gameplay transport.

## Decision

The official gameplay command envelope is:

```json
{
  "protocol_version": 1,
  "command_id": "01JZEXAMPLE01",
  "command_seq": 17,
  "client_sent_at_ms": 123456789,
  "type": "use_skill",
  "payload": {
    "skill_id": "crescent_strike",
    "target_id": "mob_1"
  }
}
```

The envelope is intentionally minimal.

## Envelope Fields

| Field | Meaning | Origin | Authority |
| --- | --- | --- | --- |
| `protocol_version` | Transport contract version expected by the client | Client | Client-declared, server-validated |
| `command_id` | Correlation identifier for a single client command attempt | Client | Client-generated, server-recorded |
| `command_seq` | Monotonic sequence number within one gameplay session | Client | Client-generated, server-validated |
| `client_sent_at_ms` | Client-side send timestamp for diagnostics and latency correlation | Client | Informational only |
| `type` | Command kind | Client | Client-declared, server-validated |
| `payload` | Command arguments | Client | Client-declared, server-validated |

## Server-Derived Context

The following values are not accepted from gameplay payloads and are derived by the server:

| Value | Source |
| --- | --- |
| `account_id` | Authenticated connection context |
| `session_id` | Active gameplay session bound to the connection |
| `actor_character_id` | Character bound to the active gameplay session |
| `region_id` | Authoritative runtime region state |
| `geodata_version` | Authoritative region terrain/geodata context |
| `authoritative_path` | Server pathfinding result for accepted movement |
| `server_received_at_ms` | Server clock |
| `applied_revision` | Server-side authoritative state progression |

## Actor Derivation

The actor of a gameplay command is always derived as:

`authenticated connection -> active gameplay session -> bound character`

The actor is never derived from `payload`.

## Rule for `character_id`

`character_id` must not exist in gameplay command envelopes.

`character_id` may exist only in pre-gameplay entry flows such as character selection or `EnterWorld` over HTTP or equivalent non-gameplay transport.

Read `docs/specs/account-auth-and-character-entry.md` for the authoritative pre-gameplay flow.

Once a gameplay session is active, the backend derives the actor character from the session binding.

## Valid Command Types

The initial command set is:

- `move_intent`
- `select_target`
- `use_skill`
- `pick_up_loot`
- `equip_item`
- `unequip_item`
- `split_item_stack`
- `merge_item_stacks`
- `buy_item`
- `exchange_item`
- `sell_item`
- `deposit_item`
- `withdraw_item`
- `offer_trade_item`
- `accept_trade_offer`
- `decline_trade_offer`
- `set_hotbar_state`

Later commands may be added by versioned extension, not by changing the semantics of existing fields.

## Examples

### Valid: `move_intent`

`move_intent` carries only the desired destination point. The server derives terrain collision, destination snapping, obstacle avoidance, and the final route from authoritative region geodata.

The client may start reversible local movement prediction when this command is dispatched, but that prediction must not add fields to the envelope and must not become gameplay truth.

```json
{
  "protocol_version": 1,
  "command_id": "01JZMOVE0001",
  "command_seq": 8,
  "client_sent_at_ms": 123456700,
  "type": "move_intent",
  "payload": {
    "point": {
      "x": 12.4,
      "z": -1.2
    }
  }
}
```

### Valid: `select_target`

```json
{
  "protocol_version": 1,
  "command_id": "01JZTARGET001",
  "command_seq": 9,
  "client_sent_at_ms": 123456740,
  "type": "select_target",
  "payload": {
    "target_id": "mob_1"
  }
}
```

### Valid: `use_skill`

```json
{
  "protocol_version": 1,
  "command_id": "01JZSKILL0001",
  "command_seq": 10,
  "client_sent_at_ms": 123456780,
  "type": "use_skill",
  "payload": {
    "skill_id": "crescent_strike",
    "target_id": "mob_1"
  }
}
```

### Valid: `split_item_stack`

```json
{
  "protocol_version": 1,
  "command_id": "01JZSPLIT001",
  "command_seq": 11,
  "client_sent_at_ms": 123456820,
  "type": "split_item_stack",
  "payload": {
    "item_instance_id": "item_duskgold_start",
    "quantity": 1
  }
}
```

### Valid: `merge_item_stacks`

```json
{
  "protocol_version": 1,
  "command_id": "01JZMERGE001",
  "command_seq": 12,
  "client_sent_at_ms": 123456860,
  "type": "merge_item_stacks",
  "payload": {
    "source_item_instance_id": "item_duskgold_split_1",
    "target_item_instance_id": "item_duskgold_start"
  }
}
```

### Valid: `buy_item`

```json
{
  "protocol_version": 1,
  "command_id": "01JZBUY0001",
  "command_seq": 13,
  "client_sent_at_ms": 123456900,
  "type": "buy_item",
  "payload": {
    "vendor_offer_id": "merchant_spear_offer",
    "quantity": 1
  }
}
```

### Valid: `deposit_item`

```json
{
  "protocol_version": 1,
  "command_id": "01JZSTORE001",
  "command_seq": 14,
  "client_sent_at_ms": 123456940,
  "type": "deposit_item",
  "payload": {
    "item_instance_id": "item_duskgold_start",
    "quantity": 1
  }
}
```

### Valid: `exchange_item`

```json
{
  "protocol_version": 1,
  "command_id": "01JZEXCHANGE1",
  "command_seq": 15,
  "client_sent_at_ms": 123456970,
  "type": "exchange_item",
  "payload": {
    "exchange_offer_id": "merchant_mantle_exchange",
    "quantity": 1
  }
}
```

### Valid: `sell_item`

```json
{
  "protocol_version": 1,
  "command_id": "01JZSELL0001",
  "command_seq": 16,
  "client_sent_at_ms": 123457000,
  "type": "sell_item",
  "payload": {
    "item_instance_id": "item_weapon_inventory",
    "quantity": 1
  }
}
```

### Valid: `withdraw_item`

```json
{
  "protocol_version": 1,
  "command_id": "01JZTAKE0001",
  "command_seq": 17,
  "client_sent_at_ms": 123457040,
  "type": "withdraw_item",
  "payload": {
    "item_instance_id": "item_warehouse_1",
    "quantity": 1
  }
}
```

### Valid: `offer_trade_item`

```json
{
  "protocol_version": 1,
  "command_id": "01JZTRADE001",
  "command_seq": 18,
  "client_sent_at_ms": 123457080,
  "type": "offer_trade_item",
  "payload": {
    "target_character_id": "char_peer_1",
    "item_instance_id": "item_duskgold_start",
    "quantity": 1
  }
}
```

### Valid: `accept_trade_offer`

```json
{
  "protocol_version": 1,
  "command_id": "01JZTRADE002",
  "command_seq": 19,
  "client_sent_at_ms": 123457120,
  "type": "accept_trade_offer",
  "payload": {
    "trade_offer_id": "trade_123"
  }
}
```

### Valid: `decline_trade_offer`

```json
{
  "protocol_version": 1,
  "command_id": "01JZTRADE003",
  "command_seq": 20,
  "client_sent_at_ms": 123457160,
  "type": "decline_trade_offer",
  "payload": {
    "trade_offer_id": "trade_123"
  }
}
```

### Valid Shape: `set_hotbar_state`

`set_hotbar_state` carries the complete effective shortcut/action bar snapshot. The backend validates active learned skills, item ownership, supported action ids, slot shape, and open-bar count before replacing the persisted character loadout.

The real payload must include exactly `36` slot entries, one for every `slot_index` from `0` to `35`. The example below is abbreviated for readability.

```json
{
  "protocol_version": 1,
  "command_id": "01JZHOTBAR01",
  "command_seq": 21,
  "client_sent_at_ms": 123457200,
  "type": "set_hotbar_state",
  "payload": {
    "open_bar_count": 2,
    "slots": [
      {
        "slot_index": 0,
        "entry_type": "skill",
        "skill_id": "crescent_strike"
      },
      {
        "slot_index": 1,
        "entry_type": "item",
        "item_instance_id": "item_duskgold_start"
      },
      {
        "slot_index": 2,
        "entry_type": "action",
        "action_id": "basic_attack"
      },
      {
        "slot_index": 3
      }
    ]
  }
}
```

## Prohibited Fields

The following fields are prohibited inside gameplay command envelopes:

- `account_id`
- `session_id`
- `character_id`
- `actor_id`
- `region_id`
- `server_received_at_ms`
- `applied_revision`
- `damage`
- `hp_after`
- `mp_after`
- `cooldown_after`
- `position_after`
- `path`
- `waypoints`
- `collision_result`
- `navigation_result`
- `navmesh_id`
- `geodata_version`
- `geodata_override`
- `known_set`
- `price`
- `item_price`
- `vendor_price`
- `tax_amount`
- `discount_amount`
- `total_cost`
- `currency_delta`

These fields are either derived by the server or are direct outcomes of authoritative rule execution.

Future economy commands must send only the identifiers and quantities needed to describe intent. The backend must derive the price and the resulting currency mutation.

Exchange commands follow the same rule: the client sends only the exchange-offer identifier and quantity. The backend derives the required materials, reward item, and resulting inventory mutation.

Storage commands follow the same rule: the client sends only item identity and quantity. The backend derives storage legality, source container truth, and resulting container mutation.

Player-trade commands follow the same rule: the client sends only the target character identifier, item identifier, quantity, or the pending trade-offer identifier. The backend derives proximity, legality, and the resulting inventory mutation.

Movement commands follow the same rule: the client sends only the intended destination point. The backend derives navigability, path, snapping, and rejection reasons.

## Idempotency Rules

### Primary Rule

Idempotency is scoped to a gameplay session.

The primary deduplication key is:

`session_id + command_seq`

### Secondary Correlation Rule

`command_id` must remain stable for retries of the same logical command.

If the same `session_id + command_seq` arrives with a different `command_id`, the command must be rejected as a protocol conflict.

### Replay Rule

If a previously processed `session_id + command_seq` is retried with the same `command_id`, the server must not reapply side effects.

The server must return the same logical outcome:

- prior `reject`, or
- prior accepted `delta` correlation path

The minimum durable record must preserve enough information to rebuild the prior outbound outcome for replay-safe retries.

The current implementation persists at least:

- `session_id`
- `command_seq`
- `command_id`
- `type`
- minimum status
- serialized outbound outcome sufficient for replay

### Ordering Rule

`command_seq` must be monotonic within a session.

Gaps, regressions, or conflicting reuse are protocol errors and must be rejected.

## Anti-Examples

### Invalid: gameplay identity in payload

```json
{
  "protocol_version": 1,
  "command_id": "01JZBAD0001",
  "command_seq": 12,
  "client_sent_at_ms": 123456999,
  "type": "use_skill",
  "payload": {
    "character_id": "char_999",
    "skill_id": "crescent_strike",
    "target_id": "mob_1"
  }
}
```

Invalid because `character_id` is not accepted in gameplay commands.

### Invalid: authoritative outcome sent by client

```json
{
  "protocol_version": 1,
  "command_id": "01JZBAD0002",
  "command_seq": 13,
  "client_sent_at_ms": 123457100,
  "type": "use_skill",
  "payload": {
    "skill_id": "crescent_strike",
    "target_id": "mob_1",
    "damage": 24
  }
}
```

Invalid because `damage` is an authoritative server outcome.

### Invalid: region declared by client

```json
{
  "protocol_version": 1,
  "command_id": "01JZBAD0003",
  "command_seq": 14,
  "client_sent_at_ms": 123457200,
  "type": "pick_up_loot",
  "payload": {
    "region_id": "gloam_field",
    "loot_id": "loot_10"
  }
}
```

Invalid because `region_id` is derived from authoritative runtime state.

`pick_up_loot` must only carry the intended `loot_id`. The client must not send local distance, region id, path, retry intent, or collection result. If the loot is known but outside immediate range, the server owns the approach path and later collection delta.

### Invalid: client-supplied item price

```json
{
  "protocol_version": 1,
  "command_id": "01JZBAD0004",
  "command_seq": 15,
  "client_sent_at_ms": 123457300,
  "type": "buy_item",
  "payload": {
    "vendor_offer_id": "offer_12",
    "quantity": 2,
    "item_price": 500,
    "total_cost": 1000
  }
}
```

Invalid because price and total cost are authoritative backend-derived values, not client truth.

### Invalid: client-supplied movement path

```json
{
  "protocol_version": 1,
  "command_id": "01JZBADMOVE1",
  "command_seq": 18,
  "client_sent_at_ms": 123457400,
  "type": "move_intent",
  "payload": {
    "point": {
      "x": 12.4,
      "z": -1.2
    },
    "waypoints": [
      {
        "x": 5,
        "z": 0
      },
      {
        "x": 12.4,
        "z": -1.2
      }
    ]
  }
}
```

Invalid because route, collision, and pathfinding outcomes are authoritative backend-derived values.

## Invariants

- The gameplay actor is derived from session binding.
- `command_seq` is monotonic per session.
- `command_id` is stable across retries of the same logical command.
- The client never sends authoritative outcomes.
- The client never sends authoritative economy values.
- The client never sends authoritative movement paths, waypoints, or collision results.
- The envelope remains transport-stable across `Fase 1.1` and `Fase 1.2`.
- Replay-safe retries are keyed by `session_id + command_seq`, with `command_id` as the stable retry identity.

## Acceptance Criteria

- All gameplay commands conform to the official envelope.
- No gameplay handler depends on client-supplied `character_id`.
- No movement handler depends on client-supplied paths, waypoints, or collision results.
- Deduplication is defined on `session_id + command_seq`.
- Conflicting replay of the same `command_seq` is rejected.
- The same envelope shape supports both `Fase 1.1` and `Fase 1.2`.
