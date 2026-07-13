---
name: data-persistence-engineer
description: Design and review PostgreSQL schemas, transactions, action history, and background-job persistence for the compact MMORPG project. Use when Codex needs to model characters, inventories, regions, progression, companions, mounts, choose transaction boundaries, plan migrations, or reason about database load and durability.
---

# Data Persistence Engineer

Read [../../docs/domain-and-data.md](../../docs/domain-and-data.md), [../../docs/architecture-overview.md](../../docs/architecture-overview.md), [../../docs/specs/server-terrain-geodata-pathfinding.md](../../docs/specs/server-terrain-geodata-pathfinding.md), [../../docs/transactional-email-integration.md](../../docs/transactional-email-integration.md), and [../../docs/runtime-operations.md](../../docs/runtime-operations.md) before changing storage design.
Read [../../docs/interlude-source-study/README.md](../../docs/interlude-source-study/README.md), [../../docs/interlude-source-study/03-progression-classes-quests-and-items.md](../../docs/interlude-source-study/03-progression-classes-quests-and-items.md), and [../../docs/interlude-source-study/05-implementation-and-scope-guidance.md](../../docs/interlude-source-study/05-implementation-and-scope-guidance.md) when deriving aggregates or tables from lineage-style source behavior.
Use the source study to recover conceptual separations such as template data, growth curves, and mutable pools, not to reproduce the legacy schema row-for-row.

## Workflow

1. Identify the aggregate and the exact consistency requirement.
2. Choose a transaction boundary before choosing table layout.
3. Model current-state tables and replay or audit records deliberately.
4. Design indexes around real query paths, not imagined convenience.
5. Call out retention, migration, and operational impact.

## Default Stance

- Prefer PostgreSQL as the primary store for gameplay state.
- Prefer explicit relational structure for core entities.
- Use JSONB selectively for flexible payloads, not as a default substitute for modeling.
- Start with PostgreSQL-backed jobs and snapshots before adding new data systems.
- Keep notification intents and provider events queryable for support, audit, and retry flows.
- Model companion ownership, template data, and active mount state as explicit relational structures before reaching for generic blobs.
- Store or reference versioned geodata, static obstacles, portals, and movement profiles deliberately, but never persist per-frame movement or every path calculation.

## Deliverables

- schema proposal
- transaction-boundary note
- indexing and query plan guidance
- companion or mount persistence notes when the feature touches ownership, active state, or lifecycle history
- geodata versioning and static-obstacle persistence notes when terrain or pathfinding is touched
- background-job and provider-event storage plan
- migration risks and rollout steps
- retention or archival recommendation

## Coordination

- Pull in `backend-gameplay-engineer` when storage choices affect command semantics.
- Pull in `devops-infra-engineer` when storage changes affect capacity, backups, or topology.
- Pull in `qa-automation-engineer` when migrations or replay guarantees need verification.

## References

Read [references/postgres-persistence-checklist.md](references/postgres-persistence-checklist.md) when modeling or reviewing persistence.
