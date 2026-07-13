# Balance and Loop Checklist

## Core Loop Questions

- What does a player try to achieve in a normal minute-to-minute session loop?
- What short-term and long-term incentives compete?
- What prevents optimal but boring farming or routing?
- What pathing, obstacle, or choke-point rule creates interesting movement without frustrating traversal?
- What makes taming, pet progression, or mount acquisition valuable without making core combat or travel loops obsolete?

## Resolution Questions

- What information is hidden, random, negotiated, or deterministic?
- In what exact order do steps resolve?
- What happens on ties, resource shortages, or invalid targets?
- How are clicked terrain points, clicked mobs, and collected AoE targets validated?
- What happens when a clicked terrain point is blocked, snapped, or unreachable?
- How are tame attempts, pet commands, summon states, and mount-state restrictions validated?

## Economy Questions

- What are the major sources and sinks for currency, consumables, gear, and upgrade materials?
- Can an undergeared player recover without invalidating good play?
- Can a geared player or farm route snowball too hard?
- Can taming, pet upkeep, or mounted traversal bypass intended economy or progression gates?

## Integrity Questions

- Can the rule be validated authoritatively on the server?
- Can movement legality be validated from server geodata rather than client collision?
- Can the result be reconstructed from saved state plus RNG inputs when needed?
- Can QA express this as a deterministic or tightly-bounded test case?
- Can multi-target and area effects be collected deterministically from the same inputs every time?
- Can companion state, pet target selection, and mounted restrictions be replayed deterministically from stored state?
