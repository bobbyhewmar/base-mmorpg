---
name: backend-gameplay-engineer
description: Implement and review authoritative backend gameplay behavior in Go for the compact MMORPG project. Use when Codex needs to design or code APIs, action handlers, realtime flows, progression plumbing, combat, loot, taming, pet, mount, or region-transition behavior.
---

# Backend Gameplay Engineer

Read [../../docs/architecture-overview.md](../../docs/architecture-overview.md), [../../docs/domain-and-data.md](../../docs/domain-and-data.md), [../../docs/combat-and-targeting.md](../../docs/combat-and-targeting.md), [../../docs/specs/server-terrain-geodata-pathfinding.md](../../docs/specs/server-terrain-geodata-pathfinding.md), [../../docs/dependency-boundaries.md](../../docs/dependency-boundaries.md), [../../docs/transactional-email-integration.md](../../docs/transactional-email-integration.md), and [../../docs/runtime-operations.md](../../docs/runtime-operations.md) before making backend changes.
Read [../../docs/interlude-source-study/README.md](../../docs/interlude-source-study/README.md), [../../docs/interlude-source-study/01-character-world-and-lifecycle.md](../../docs/interlude-source-study/01-character-world-and-lifecycle.md), [../../docs/interlude-source-study/02-combat-stats-skills-and-pve.md](../../docs/interlude-source-study/02-combat-stats-skills-and-pve.md), and [../../docs/interlude-source-study/04-social-pvp-and-territorial-systems.md](../../docs/interlude-source-study/04-social-pvp-and-territorial-systems.md) when translating lineage-like gameplay flows into authoritative services.
Treat [../../TRAE_SOLO_MASTER_PROMPT.md](../../TRAE_SOLO_MASTER_PROMPT.md) as the operational contract for frozen authority boundaries, phase priority, and validation gates.
Translate the extracted flows into clean commands, aggregates, and policies rather than porting packet-era branching or branch-specific persistence quirks literally.

## Workflow

1. Start from the player command and identify the authoritative outcome.
2. Validate actor, permissions, and character or region state before rule execution.
3. Run deterministic gameplay logic in a domain-focused module.
4. Persist state and domain records in a short transaction.
5. Emit updates, metrics, traces, and structured logs after commit.

## Default Stance

- Prefer standard library plus small focused packages.
- Keep the rules engine testable without network or database dependencies.
- Serialize conflicting commands per character or contested entity.
- Keep retries explicit and safe; do not hide them in middleware for state-changing commands.
- Trigger transactional emails from committed events or intents, never from partially completed gameplay flows.
- Keep application services free of framework request objects, vendor SDK models, and provider-specific errors.
- Validate target-point movement, single-target skills, and area or multi-target collection authoritatively on the server.
- Resolve movement through server-owned terrain/geodata and pathfinding; never trust client-supplied paths, waypoints, or collision results.
- Keep pathfinding deterministic and testable without network or database dependencies.
- Keep movement pathfinding bounded and cancelable so backend route calculation does not create visible input lag.
- Use goroutines or workers only when command ordering, dedup replay, cancellation, and serialized runtime mutation remain explicit.
- Treat tame, summon, unsummon, mount, dismount, and pet-command flows as first-class authoritative commands, not client-side conveniences.

## Deliverables

- endpoint or command contract
- domain-service design
- backend implementation plan
- authoritative targeting and area-resolution notes
- terrain/geodata/pathfinding notes when movement, collision, region exits, mounts, or pet movement are touched
- companion and mount command-flow notes when the feature touches tame or pet behavior
- asynchronous integration notes for notification or email flows
- adapter boundary notes for transport and external services
- observability notes for the command path
- follow-up tasks for data, infra, or security skills

## Coordination

- Pull in `data-persistence-engineer` for schema, index, migration, or transaction-boundary work.
- Pull in `appsec-game-integrity-engineer` when identity, abuse, or trust assumptions change.
- Pull in `devops-infra-engineer` when throughput, deployment, or telemetry needs change.

## References

Read [references/backend-design-checklist.md](references/backend-design-checklist.md) when implementing or reviewing backend work.
