# PostgreSQL Persistence Checklist

## Modeling

- Which tables hold current truth?
- Which tables hold command history, event history, or snapshots?
- Which columns must be indexed for the primary character, region, inventory, and progression queries?
- Which columns must be indexed for active companion, mount-state, and tameable-species lookups?
- Which notification-intent and provider-event tables must stay queryable for support and retries?
- Which columns must be indexed for item-instance owner, container membership, equip-slot occupancy, and template-based inventory queries?
- Which geodata, static-obstacle, portal, and movement-profile records are versioned content versus hot gameplay state?

## Transactions

- What commits atomically?
- What can move to post-commit jobs?
- What idempotency key protects client retries?
- Does taming atomically convert the attempted outcome into owned companion state without duplicate acquisition on retry?
- Does a trade, exchange, or storage transfer commit all involved item-instance moves atomically without duplicating or orphaning fragments?
- Does movement avoid durable writes for every path calculation, waypoint advance, or frame?

## Load Safety

- Can this query be bounded by character, region, account, or time?
- Can the query avoid full-table scans on hot paths?
- Can a background job compete with live gameplay unless throttled?
- Can companion or mount reads stay bounded by owner, region, or active state on hot gameplay paths?
- Can inventory restore and storage restore stay bounded by owner, container, and slot without scanning all item rows?
- Can geodata reads be loaded or cached by region/version instead of queried per movement frame?

## Operability

- How is the change migrated safely?
- How is old data retained, archived, or compacted?
- Which metrics reveal lock contention, slow queries, or pool pressure?
- How are email intents, message records, and webhook events retained without bloating hot tables?
- How are retired companions, released pets, or mount-state history retained without bloating hot current-state tables?
- How are destroyed items, transfer records, or exchange audits retained without polluting hot inventory truth tables?
- How are retired geodata versions retained or migrated without invalidating active region runtime unexpectedly?
