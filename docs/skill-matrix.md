# Skill Matrix

## Team Shape

Use skills as role lenses, not as a promise of headcount. One person may cover several skills, but each skill should still produce explicit outputs and handoffs.
The gameplay reference dossier lives under [interlude-source-study](interlude-source-study/README.md) and should guide concept extraction, documentation updates, and skill guidance while implementation stays aligned with our architecture.
When a skill uses the source study, its output should become project-specific specs or guidance, not a literal porting plan.
When work is being executed autonomously inside this repository, [../TRAE_SOLO_MASTER_PROMPT.md](../TRAE_SOLO_MASTER_PROMPT.md) is the operational contract for priority order, validation gates, and stop criteria.

## Skills

### Product Game Director

- Own scope, priorities, milestone slicing, and tradeoff framing.
- Produce milestone plans, acceptance criteria, and defer lists.

### Systems Designer

- Own mechanics, pacing, balance, and rule clarity.
- Produce combat, progression, region-loop specs, companion or mount rules, state transitions, and balance assumptions.

### Solution Architect

- Own system boundaries, contracts, and non-functional tradeoffs.
- Produce architecture decisions, dependency rules, runtime-boundary rules, terrain/pathfinding boundaries, and flow diagrams.

### Three.js Client Engineer

- Own the world client, interaction model, rendering structure, and the boundary between scene and HUD.
- Produce scene architecture, HUD behavior, selection flows, pending versus authoritative path presentation, companion or mount interaction flows, platform-bridge boundaries, client implementation plans, and faithful visual adaptation of the compact MMORPG direction.

### Backend Gameplay Engineer

- Own authoritative APIs, rule execution plumbing, realtime flows, and service behavior in Go.
- Produce module designs, handlers, command flows, server-side geodata/pathfinding behavior, companion or mount lifecycle behavior, and backend code changes.

### Data Persistence Engineer

- Own schema design, transaction boundaries, indexing, migrations, and replay storage.
- Produce data models, query plans, geodata versioning and static-obstacle persistence structure, companion persistence structure, and storage migration decisions.

### DevOps Infra Engineer

- Own Linux production, deployment paths, observability, and runtime readiness.
- Produce environment topology, SLO drafts, runbooks, and deployment safeguards.

### AppSec and Game Integrity Engineer

- Own trust boundaries, abuse resistance, data protection, and authoritative integrity controls.
- Produce threat reviews, control checklists, and hardening priorities.

### QA Automation Engineer

- Own test strategy, simulation coverage, release gates, and measurable quality.
- Produce test plans, load scenarios, pathfinding and collision regressions, companion or mount regressions, and automation guidance.

## Typical Handoffs

- Product Game Director -> Systems Designer for mechanic clarification
- Systems Designer -> Solution Architect for implementation constraints
- Solution Architect -> Client, Backend, and Data skills for execution
- AppSec and QA -> every implementation skill before release
- DevOps Infra -> Backend and Data when production behavior changes
