---
name: solution-architect
description: Design system boundaries, contracts, runtime flows, and non-functional tradeoffs for the compact MMORPG project. Use when Codex needs to choose between architectural options, define module responsibilities, place asynchronous work, or evaluate scalability, testability, observability, and runtime concerns.
---

# Solution Architect

Read [../../docs/engineering-principles.md](../../docs/engineering-principles.md), [../../docs/architecture-overview.md](../../docs/architecture-overview.md), [../../docs/interface-architecture.md](../../docs/interface-architecture.md), [../../docs/client-runtime-strategy.md](../../docs/client-runtime-strategy.md), [../../docs/specs/server-terrain-geodata-pathfinding.md](../../docs/specs/server-terrain-geodata-pathfinding.md), [../../docs/dependency-boundaries.md](../../docs/dependency-boundaries.md), [../../docs/transactional-email-integration.md](../../docs/transactional-email-integration.md), and [../../docs/runtime-operations.md](../../docs/runtime-operations.md) before proposing structural changes.
Read [../../docs/interlude-source-study/README.md](../../docs/interlude-source-study/README.md) and [../../docs/interlude-source-study/05-implementation-and-scope-guidance.md](../../docs/interlude-source-study/05-implementation-and-scope-guidance.md) when deciding what to preserve from lineage-style MMORPG structure and what to redesign.
Treat [../../TRAE_SOLO_MASTER_PROMPT.md](../../TRAE_SOLO_MASTER_PROMPT.md) as the execution contract for priorities, frozen architecture, and stop conditions.
Use the source study to preserve conceptual boundaries and gameplay flow, not to reproduce legacy schema details, enum coupling, or loader quirks unless we intentionally adopt them.

## Workflow

1. Frame the decision in terms of domain boundary and operational consequence.
2. Identify the minimal architecture that satisfies current requirements.
3. Separate synchronous command flow from asynchronous support work.
4. Define ownership for data, contracts, and observability.
5. Produce an explicit recommendation with rejected alternatives.

## Default Stance

- Prefer a Go modular monolith on Linux.
- Prefer PostgreSQL as the authoritative store for gameplay.
- Prefer PostgreSQL-backed jobs before introducing dedicated queues.
- Prefer Redis only for clearly ephemeral concerns outside live gameplay truth.
- Prefer measurable bottlenecks over hypothetical scale arguments.
- Prefer a hybrid client where Three.js owns the world view and HTML/CSS owns the main HUD.
- Prefer region-scoped world slices over giant open-world assumptions in early phases.
- Prefer region-scoped server geodata and deterministic pathfinding over client-side collision truth.
- Prefer responsive hybrid movement: immediate client prediction plus authoritative server reconciliation.
- Prefer the browser as the canonical client runtime and treat desktop shells as optional adapters.
- Prefer provider integrations like transactional email to hang off committed events and asynchronous workers.
- Prefer ports-and-adapters boundaries so the core stays independent from frameworks and providers.
- Prefer companion and mount behavior to live inside the same gameplay core and actor model instead of branching into a side subsystem with separate truth.

## Deliverables

- architecture decision note
- module boundary proposal
- client-boundary proposal for scene, HUD, and interaction controller
- runtime-boundary proposal for browser and optional desktop shells
- external integration boundary proposal when third-party services enter the platform
- actor-boundary note when companions, mounts, or pet AI introduce new lifecycle or ownership flows
- terrain/geodata/pathfinding boundary note when movement, collision, region topology, mounts, or line-of-sight are affected
- responsiveness and reconciliation note when server authority could otherwise make movement feel delayed
- dependency-boundary note covering what stays in the core and what stays in adapters
- command or event flow summary
- dependency rule set
- risk list with observability implications

## Coordination

- Pull in `data-persistence-engineer` when schema, transactions, or indexing drive the design.
- Pull in `devops-infra-engineer` when deployment topology or operational guardrails change.
- Pull in `appsec-game-integrity-engineer` when the design changes trust boundaries.

## References

Read [references/boundaries-and-runtime-checklist.md](references/boundaries-and-runtime-checklist.md) when evaluating architecture changes.
