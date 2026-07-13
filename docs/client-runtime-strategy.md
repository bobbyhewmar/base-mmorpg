# Client Runtime Strategy

## Direction

Adopt a `web-first` client strategy.

The canonical client runtime is the browser. Desktop packaging may be added later as an optional shell, not as the primary architectural assumption.

## Why Web First

- zero-install access lowers friction for players
- deployment and updates stay simple
- the current client stack already fits the web naturally
- observability and rollback are easier with a single web deployment path
- this reduces coupling to a desktop runtime early in the project

## Runtime Rule

The client application must not be coupled to a specific runtime such as browser-only APIs, Electron-specific bridges, or Tauri-specific commands in the core client flow.

Instead:

- the main client application owns scene, HUD, state, and interaction logic
- a thin platform bridge owns runtime-specific capabilities
- runtime adapters implement that bridge for browser first and desktop shells later

## Canonical Runtime

### Browser

Treat the browser as the primary target for:

- gameplay
- account access
- account registration
- character selection and entry
- character creation
- live updates
- ordinary support and session flows

The browser is the delivery runtime, not a trusted authority.

The browser client may collect:

- credentials
- character-creation choices
- movement destination and combat intent
- inventory and interaction intent

The browser client must not own authoritative business truth of any kind. It is a presenter, input collector, and reconciliation surface only.

For movement, the browser may own immediate local prediction and animation responsiveness. That prediction is presentation, not durable or authoritative gameplay state.

The browser client must not be the source of truth for:

- authenticated identity
- active gameplay actor
- character name availability
- race or base-class legality
- item prices or economy values
- gameplay results
- terrain collision, pathfinding, or final route decisions
- persisted inventory, equipment, or progression outcomes

## Optional Future Runtimes

### Desktop Shell

If a desktop client becomes valuable later, add it as a wrapper around the web client using a platform adapter.

Possible reasons to add one later:

- desktop notifications beyond normal browser behavior
- local file integration
- richer native presence integration
- installer-driven distribution goals

### Decision Rule

Do not adopt a desktop shell until a specific product or platform need justifies the extra runtime and release complexity.

## Platform Bridge

Define a small internal bridge for capabilities that may vary by runtime, such as:

- notifications
- deep links
- clipboard or share helpers
- external link handling
- storage extensions if ever needed

The game UI and interaction flow should call internal client services, not runtime APIs directly.

## What Must Stay Runtime-Neutral

- scene composition
- HUD composition
- login, registration, and character-entry UI flows
- selection flow
- state synchronization
- action confirmation flow
- command dispatch to backend

## Security Rule

- Treat every client request as hostile until validated on the backend.
- Use HTTPS for account flows and WSS for online gameplay sessions.
- Keep session tokens, attach tokens, and account recovery tokens server-issued and server-validated.
- Do not trust browser-side hidden fields, cached catalogs, or displayed prices as authoritative inputs.
- Future vendor, trade, or economy requests must send identifiers and quantities only; the backend must derive the price and resulting currency mutation.

## What May Live In Runtime Adapters

- desktop shell bootstrapping
- browser-specific install prompts
- runtime-specific notification permissions
- native windowing integration
- platform-specific deep-link registration

## Packaging Bias

- browser first
- optional desktop shell second
- no hard requirement on Electron
- no hard requirement on Tauri

Choose the shell later based on real product needs, not as a foundational constraint today.
