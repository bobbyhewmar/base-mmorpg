# Dependency Boundaries

## Rule

The application core must not be coupled to a framework, SDK, or external service provider.

Only adapters at the edge of the system may know about concrete libraries, protocols, or vendor-specific models.

## Architectural Shape

Use a ports-and-adapters style inside the modular monolith:

- domain and application layers define internal contracts
- adapters implement those contracts for HTTP, WebSocket, PostgreSQL, email providers, optional pathfinding libraries, and other external systems
- a composition root wires concrete implementations together

## What The Core May Depend On

- domain models
- application commands and queries
- internal interfaces owned by this codebase
- simple standard-library primitives when they do not leak framework semantics

## What The Core Must Not Depend On

- HTTP framework request or response objects
- WebSocket library event models
- vendor SDK request or response types
- ORM-specific entities or decorators
- provider-specific error taxonomies outside adapter translation
- third-party navmesh or pathfinding library types outside an internal terrain/pathfinding port

## Adapter Rules

### Own The Contract Internally

- Define interfaces in the application boundary, not in the vendor package.
- Name interfaces after our use case, not after the provider.
- Keep method signatures focused on domain inputs and outputs.

### Translate At The Edge

- Convert internal DTOs to provider payloads inside adapters.
- Convert provider errors into internal error categories inside adapters.
- Convert webhook payloads into internal events before the rest of the application sees them.

### Keep Swapping Realistic

- The goal is substitutability, not fantasy abstraction.
- Make it easy to swap providers that solve the same problem.
- Do not create giant universal interfaces that pretend all providers behave identically.
- Add only the contract surface the current use cases need.

## Framework Boundary Rules

### HTTP And WebSocket

- Handlers may use a library or router, but application services must not.
- Parse transport details at the edge and pass plain internal commands inward.
- Return internal outcomes and map them back to transport responses at the edge.

### Background Jobs

- Workers may use a scheduler or queue implementation.
- Job execution logic must depend on internal services, not the queue library itself.

### Persistence

- Repositories may use SQL drivers or query tooling.
- Domain logic must not depend on ORM annotations or query builder types.
- Keep transaction control behind an internal unit-of-work or repository boundary when needed.

### Terrain And Pathfinding

- Gameplay services may depend on internal `GeodataProvider` or `Pathfinder` style ports.
- If a third-party navmesh or pathfinding library is introduced later, keep it behind an adapter.
- Do not let library-specific graph, polygon, or node types leak into command handlers or domain rules.
- The first implementation should prefer a small project-owned deterministic pathfinder unless measured need justifies a heavier dependency.

### Third-Party Providers

- Email, storage, auth, analytics, and similar services must sit behind internal provider ports.
- Keep provider-specific configuration and credentials in the composition root and runtime environment only.

## Example: Transactional Email

Good boundary:

- application creates a `notification_intent`
- application calls an internal `EmailSender` or `NotificationDispatcher` port
- `resend` adapter maps the internal message into the Resend SDK or API payload

Bad boundary:

- gameplay service imports the Resend SDK directly
- application logic builds Resend payload objects
- provider webhook JSON shapes leak into domain code

## Testing Implications

- Test the application layer with fake internal ports.
- Contract-test adapters against the provider behavior they wrap.
- Test terrain/pathfinding rules through deterministic fixtures before adding renderer or database concerns.
- Keep end-to-end tests focused on integration seams rather than every rule path.

## Decision Filter

Before accepting a dependency, ask:

1. Does the core know this dependency exists?
2. Can we swap this provider by replacing one adapter and configuration wiring?
3. Do vendor types leak past the adapter boundary?
4. Would a framework upgrade force domain or application changes?
5. Is the abstraction owned by us and shaped around our use case?
