# Quality Strategy Checklist

## Rule Coverage

- Which rules deserve unit tests?
- Which state transitions deserve property or simulation tests?
- Which bugs would be expensive to discover only in staging or production?
- Which target-point, target-entity, AoE, and multi-target edge cases deserve deterministic tests first?
- Which terrain/geodata/pathfinding edge cases deserve deterministic fixtures first?
- Which click-to-move responsiveness cases prove the player does not wait for server pathfinding?
- Which inventory transitions such as stack split, equip swap, trade commit, storage transfer, and destroy flow deserve deterministic tests first?

## Integration Coverage

- Which API contracts need end-to-end validation?
- Which database migrations need rehearsal on representative data?
- Which realtime flows need multi-client scenarios?
- Which movement, targeting, and cast-preview flows need client-plus-backend validation?
- Which blocked destination, alternate route, snap, and unreachable movement flows need client-plus-backend validation?
- Which prediction-to-authoritative-route reconciliation cases need visual or read-model tests?
- Which inventory, equipment, storage, and vendor flows need client-plus-backend validation?

## Load and Resilience

- What command mix should the load test simulate?
- What pathfinding complexity, obstacle density, and movement rejection mix should the load test simulate?
- What happens under pool pressure, lock contention, or reconnect storms?
- Which metrics define pass or fail?

## Release Confidence

- Which checks run on every pull request?
- Which checks run before deploy?
- Which dashboards or alerts confirm a healthy release?
