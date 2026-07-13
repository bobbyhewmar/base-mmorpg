# Boundaries and Runtime Checklist

## Boundary Questions

- Which module owns this decision?
- Which data must stay transactionally consistent?
- Which information is derived and can be rebuilt?
- If this touches movement, which part is client presentation and which part is server geodata/pathfinding authority?
- How does the design avoid making the player wait motionless for authoritative pathfinding?

## Runtime Questions

- Is this path synchronous or asynchronous?
- What is the latency budget?
- What happens when the dependency is slow or unavailable?
- What are the pathfinding budget limits, and how does the system fail closed when they are exceeded?
- What is the prediction leash and reconciliation strategy under normal and high latency?

## Operational Questions

- Which metrics prove this design works?
- Which logs or traces explain failure?
- Which limits prevent one region, character, or user from harming the whole system?
- Which movement rejection reasons and pathfinding latency signals are observable?

## Simplicity Questions

- Can this stay in the monolith?
- Can PostgreSQL handle this cleanly?
- Does Redis or a queue solve a real measured problem or an imagined one?
