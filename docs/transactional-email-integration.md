# Transactional Email Integration

## Direction

Integrate transactional email as a first-class platform capability, starting with `Resend`, but keep it outside the critical gameplay command path.

The application core must know about an internal email or notification port, not about Resend directly.

## Product Use Cases

Start with emails that support trust, retention, and coordination:

- account verification
- password reset or account recovery
- email change confirmation
- security alerts such as new login or suspicious account activity
- optional social or guild invites later if the product needs them

Do not send promotional or broad marketing campaigns through the same gameplay-triggered workflow.

## Architectural Position

### Core Rule

Gameplay commands must never depend on a live email provider call to succeed.

### Flow

1. A gameplay or account flow commits its authoritative state change.
2. The same transaction writes a notification intent or outbox record.
3. A background worker claims pending email jobs from PostgreSQL.
4. The worker calls an internal provider port and an adapter renders or references the correct template and sends through Resend.
5. The worker stores provider identifiers and immediate send results.
6. A webhook endpoint receives delivery events from Resend.
7. The backend verifies the webhook signature and stores delivery outcomes for observability and support workflows.

## Recommended Module Split

### In-Process Notification Module

Own:

- notification policy
- template selection
- deduplication rules
- cooldown rules
- recipient resolution

### Background Delivery Worker

Own:

- job claiming
- retries for transient provider failures
- send execution through an internal email-provider port
- status transitions for queued, sent, failed, and terminal states

### Webhook Ingest Endpoint

Own:

- signature verification
- idempotent event ingestion
- delivery, bounce, open, click, and complaint status capture when used

## Data Model Bias

Start with separate persistence concerns:

- `notification_intents`
- `email_messages`
- `email_provider_events`
- `jobs`

Suggested meanings:

- `notification_intents`: business-level intent such as `email_change_confirmation` or `suspicious_login`
- `email_messages`: actual provider send attempts and message identifiers
- `email_provider_events`: webhook outcomes and provider metadata

## Idempotency And Dedupe

- Deduplicate by business event plus recipient plus template family where appropriate.
- Keep a stable idempotency key for every notification intent.
- Allow some notifications to collapse into one active message, such as repeated suspicious-login notices inside a short cooldown window if policy allows it.
- Keep security alerts and password-reset emails non-collapsible.

## Template Strategy

Prefer hosted transactional templates referenced by template id plus variables. This keeps message design editable without redeploying the gameplay service.

Organize template families like:

- `account-verification`
- `password-reset`
- `email-change-confirmation`
- `security-alert`
- `guild-invite` if the product adds that flow

## Domain And Sender Strategy

- Use a verified sending subdomain, not the root domain.
- Keep sender intent explicit, for example `noreply@notifications.example.com` or `security@notifications.example.com`.
- Separate security-oriented senders from general gameplay notifications if needed later.

## Security And Integrity

- Keep the Resend API key in secret storage only.
- Verify webhook signatures with the raw request body.
- Treat provider webhooks as untrusted until verified and persisted.
- Keep audit trails for account and security-related notifications.
- Keep Resend-specific payload handling inside the Resend adapter boundary.

## Provider Boundary

### Internal Ports

Shape ports around our use cases, for example:

- send transactional email
- record provider delivery event
- resolve message status

### Adapter Rule

- `Resend` is the first adapter, not a permanent architectural truth.
- Do not let Resend SDK models, template objects, or webhook payload structs leak into application services.
- Keep the swap cost localized to adapter code, configuration, and contract tests.

## Observability

Track at least:

- notification intents created
- queued emails
- send success and failure counts
- provider response latency
- bounce and complaint counts
- webhook ingest failures
- notification lag from game event to provider send

## Failure Policy

- If Resend is temporarily unavailable, keep the gameplay action successful and the email pending.
- Retry transient send failures through the worker with bounded backoff.
- Mark non-recoverable failures explicitly for support visibility.
- Do not loop retries forever without escalation.

## MVP Scope

Implement first:

1. account verification
2. password reset
3. email change confirmation
4. suspicious login alert

Delay more advanced automations until basic delivery, observability, and support tooling work well.
