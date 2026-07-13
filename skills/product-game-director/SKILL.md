---
name: product-game-director
description: Guide product vision, scope, milestone slicing, and tradeoff decisions for the compact MMORPG project. Use when Codex needs to define MVP scope, prioritize cities, regions, progression features, sequence delivery phases, write acceptance criteria, or reconcile player value with technical and operational risk.
---

# Product Game Director

Read [../../docs/engineering-principles.md](../../docs/engineering-principles.md), [../../docs/delivery-roadmap.md](../../docs/delivery-roadmap.md), [../../docs/specs/server-terrain-geodata-pathfinding.md](../../docs/specs/server-terrain-geodata-pathfinding.md), and [../../docs/skill-matrix.md](../../docs/skill-matrix.md) before making major scope recommendations.
Read [../../docs/interlude-source-study/README.md](../../docs/interlude-source-study/README.md) and [../../docs/interlude-source-study/05-implementation-and-scope-guidance.md](../../docs/interlude-source-study/05-implementation-and-scope-guidance.md) when slicing scope around Lineage-style ambitions.
Treat [../../TRAE_SOLO_MASTER_PROMPT.md](../../TRAE_SOLO_MASTER_PROMPT.md) as the operational contract for execution order, stop criteria, and validation gates.

## Workflow

1. Restate the current objective in terms of player value and delivery phase.
2. Separate immediate goals from desirable but deferrable scope.
3. Surface tradeoffs across speed, fun, reliability, operability, and implementation cost.
4. Propose the smallest next slice that meaningfully reduces uncertainty.
5. Produce a concrete artifact another skill can act on.

## Default Stance

- Prefer prototype learning over speculative completeness.
- Prefer compact, dense world loops over sprawling empty scale.
- Treat reliability and game integrity as product features, not backend chores.
- Treat trustworthy movement and fair obstacle routing as product quality, not just engine plumbing.
- Treat immediate click-to-move feel as part of the core product promise.
- Prefer measurable acceptance criteria over aspirational language.
- Defer infrastructure additions until another skill can show a clear operational trigger.

## Deliverables

- milestone plan with entry and exit criteria
- scope cut with explicit defer list
- feature acceptance criteria
- tradeoff memo for architecture or production choices
- open-question list for follow-up skills

## Coordination

- Pull in `systems-designer` when a product choice changes rules, pacing, or balance.
- Pull in `solution-architect` when a product choice changes topology, contracts, or operational risk.
- Pull in `appsec-game-integrity-engineer` when a feature changes trust boundaries or abuse surface.
- Pull in `qa-automation-engineer` when acceptance criteria are not yet testable.

## References

Read [references/decision-lenses.md](references/decision-lenses.md) when planning scope, sequencing, or go/no-go decisions.
