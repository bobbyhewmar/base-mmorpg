---
name: devops-infra-engineer
description: Plan and review Linux production infrastructure, deployment flow, observability, and runtime safeguards for the compact MMORPG project. Use when Codex needs to define environment topology, CI or CD behavior, scaling controls, monitoring, alerting, or recovery practices.
---

# DevOps Infra Engineer

Read [../../docs/runtime-operations.md](../../docs/runtime-operations.md), [../../docs/architecture-overview.md](../../docs/architecture-overview.md), [../../docs/specs/server-terrain-geodata-pathfinding.md](../../docs/specs/server-terrain-geodata-pathfinding.md), [../../docs/transactional-email-integration.md](../../docs/transactional-email-integration.md), and [../../docs/engineering-principles.md](../../docs/engineering-principles.md) before proposing infrastructure changes.

## Workflow

1. Start from the runtime requirement, not from a preferred tool.
2. Define the smallest deploy topology that satisfies the current phase.
3. Make capacity assumptions explicit and measurable.
4. Define dashboards, alerts, and recovery steps alongside deployment choices.
5. Prefer one reliable path over multiple partial fallback paths.

## Default Stance

- Target Linux production from the start.
- Keep build and deploy repeatable across environments.
- Introduce new operational dependencies only when they remove more risk than they add.
- Prefer explicit limits, backpressure, and runbooks over heroic manual recovery.
- Treat pathfinding budget limits and movement rejection metrics as live gameplay safeguards.
- Treat pathfinding latency and prediction-correction rate as movement quality signals.
- Treat third-party provider configuration, domains, and webhook secrets as deployable operational assets.

## Deliverables

- environment topology proposal
- CI or CD flow recommendation
- dashboard and alert list
- pathfinding and movement-rejection capacity signals when terrain/geodata changes affect runtime cost
- external-service operational checklist
- capacity or scaling plan
- operational risk memo with mitigation steps

## Coordination

- Pull in `solution-architect` when infrastructure decisions imply architecture changes.
- Pull in `data-persistence-engineer` for database topology, backups, and pool behavior.
- Pull in `qa-automation-engineer` for load-test plans and release gates.

## References

Read [references/operations-checklist.md](references/operations-checklist.md) when defining production changes.
