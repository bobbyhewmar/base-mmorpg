# Engineering Principles

Use these principles across every skill and implementation decision.

## Primary Goals

- Keep the system simple enough to reason about under stress.
- Keep game rules deterministic and testable outside transport and storage layers.
- Keep runtime behavior observable and measurable in production.
- Keep operational dependencies proportional to proven need.

## Core Rules

### Treat source studies as concept input

- Use studied codebases to extract concepts, state transitions, responsibility splits, and useful constraints.
- Do not copy branch-specific ids, packet shapes, schema quirks, enum coupling, or loader hacks by default.
- Turn extracted findings into project-specific documentation, skills, and implementation guidance before building.
- Only adopt a source-specific detail when we intentionally decide it helps our game and record that choice clearly.

### Prefer one source of truth per concern

- Keep gameplay authority on the backend, not in the client.
- Keep movement collision, pathfinding, and terrain navigability on the backend, not in the client.
- Keep client-side movement prediction reversible and bounded instead of turning responsiveness into a second authority.
- Keep account identity, character creation, and economy valuation on the backend, not in the client.
- Keep durable state in PostgreSQL unless there is a measured reason to split it.
- Keep ephemeral state explicitly ephemeral. Presence, throttling, and fan-out may use Redis later, but not as live gameplay truth.

### Prefer a modular monolith first

- Start with one deployable backend written in Go.
- Separate modules by domain boundaries, not by network calls.
- Introduce service splits only after a measured scaling or team-boundary need.

### Prefer synchronous, explicit critical paths

- Run player commands through one authoritative request path.
- Persist state changes and emit domain events in the same transactional flow when possible.
- Reject invalid commands explicitly instead of hiding problems behind retries or soft fallbacks.

### Avoid fallback culture

- Do not build parallel truth sources for the same game decision.
- Do not add caches to the write path.
- Do not add retries around non-idempotent operations without a clear idempotency contract.
- Do not silently degrade security or consistency to preserve convenience.
- Do not trust client-supplied prices, totals, or identity fields as authoritative shortcuts.
- Do not trust client-supplied paths, waypoints, or collision results as authoritative shortcuts.
- Do not invent canonical catalog options, character templates, skills, items, regions, or rules in the client when the authoritative source is missing.
- Treat missing canonical data as an explicit blocked or rejected state with visible UI and stable reason codes, not as an opportunity to substitute a convenient default.

### Design for measurement before scale

- Add metrics, traces, and structured logs before adding new infrastructure tiers.
- Define capacity assumptions, then load-test them.
- Make every operational promise observable through a dashboard or query.
- Measure pathfinding latency and movement rejection reasons before adding heavier terrain infrastructure.
- Measure click-to-move perceived latency so pathfinding correctness does not make movement feel stuck.

### Keep abstractions honest

- Extract interfaces when they reduce coupling or enable testing, not by default.
- Prefer standard library and small dependencies over framework-heavy stacks.
- Keep commands, events, and persisted records named after domain concepts.

### Keep dependencies replaceable

- Keep framework and vendor knowledge at the system edge.
- Let the application core depend on internal ports, not SDKs or framework objects.
- Translate provider payloads, webhook bodies, and framework models inside adapters only.
- Wire concrete implementations in one composition root so replacements stay local.

Read [dependency-boundaries.md](dependency-boundaries.md) when introducing a new framework, library, or provider.

## Decision Filters

Use these questions before approving a design:

1. Does this reduce complexity more than it adds?
2. Does this make the rule path more testable?
3. Can we observe and explain failure modes in production?
4. Is the operational cost justified by measured need?
5. Can another skill implement or verify this without hidden context?

## Default Technology Bias

- Runtime: Linux
- Backend: Go
- Primary client runtime: Web browser
- Primary database: PostgreSQL
- Queueing: PostgreSQL job tables first
- Search: PostgreSQL full-text search first
- Redis: Optional for ephemeral concerns after need is clear
