# Operations Checklist

## Deploy Path

- Is there one clear build path?
- Is there one clear deploy path?
- Are configuration and secrets separated from code?
- Are provider API keys, sending domains, and webhook secrets managed outside the repository?
- Can providers or adapters be swapped through configuration and composition without rewriting the core?

## Observability

- Which dashboards prove the service is healthy?
- Which alerts page the team before players notice?
- Which logs and traces help explain a failed player action or region transition?
- Which metrics explain pathfinding latency, budget exhaustion, blocked movement, and unreachable destinations?
- Which metric or test catches a click-to-move delay that feels like server lag?
- Which dashboards prove transactional emails are being queued, sent, and confirmed correctly?

## Capacity

- What is the expected command rate, not just user count?
- What is the expected pathfinding cost per region and obstacle density?
- What protects PostgreSQL from connection storms?
- What bounds worker concurrency and queue growth?

## Recovery

- How is rollback or forward-fix handled?
- How is database restore rehearsed?
- Which dependencies can fail without blocking live sessions?
- How are missed provider webhook events replayed or reconciled?
