# Backend Design Checklist

## Command Path

- Who can issue this command?
- What character, session, or region state must exist first?
- What makes the command invalid?
- Which transport details must be stripped away before the application layer runs?
- Is this a target-point command, target-entity command, or area-selection command?
- If this is a target-point movement command, is the route derived from server geodata instead of client waypoints?
- If pathfinding may be slow, what budget, cancellation, or worker boundary prevents visible input lag and stale outcomes?
- Is this a companion-lifecycle command such as tame, summon, unsummon, mount, dismount, or pet action?
- Is this an inventory transition such as equip, unequip, split stack, drop, destroy, storage transfer, trade commit, or exchange commit?

## Rule Execution

- Can the rule run without I/O?
- Can the result be reproduced in a test from fixtures plus RNG input?
- Does the code expose the difference between validation failure and system failure?
- Does target collection for AoE or multi-target skills stay deterministic and bounded?
- Does pathfinding stay deterministic, bounded, and unable to cut through blockers?
- Does async pathfinding preserve command ordering, dedup replay, and serialized runtime mutation?
- Are companion ownership checks, active-companion limits, and mount restrictions enforced in explicit rule paths?
- Are item-template conditions, instance legality, and container-specific restrictions separated cleanly instead of mixed into one handler?

## Persistence and Events

- Which records must commit atomically?
- Which side effects can happen only after commit?
- Which identifiers make retries safe?
- Which runtime movement state is ephemeral, and which geodata/version data is durable or content-backed?

## Boundaries

- Do framework request or response models leak into the application layer?
- Do provider SDK structs or errors leak past adapters?
- Do navmesh/pathfinding library types leak past an internal geodata/pathfinder boundary?
- Is the command handled through internal ports that another adapter could reuse?

## Observability

- Which metric counts success and rejection reasons?
- Which trace span covers rule execution and persistence?
- Which log fields explain actor, character, region, command, and outcome?
- Which metric counts pathfinding latency, budget failures, and movement rejects by reason?
- Which log fields explain companion instance, monster species, or mounted state when those are part of the command?
- Which log fields explain item instance, template id, source container, target container, quantity, and outcome for inventory-changing commands?
