# Decision Lenses

## Use These Lenses In Order

### Player Value

- Does the change make the game clearer, more engaging, or more replayable?
- Does it remove confusion from the core loop?
- Does movement through terrain feel fair, readable, and consistent with what the player sees?
- Does click-to-move feel immediate, or does server authority create visible input lag?

### Learning Speed

- Does this help the team validate a risky assumption quickly?
- Can the team learn the same thing with a smaller slice?

### Delivery Cost

- Does this pull in new infrastructure, assets, or operational work?
- Does this create a dependency chain that blocks other roles?

### Operational Risk

- Does this add a new live dependency or failure mode?
- Does this make observability or recovery harder?
- Does this introduce pathfinding cost or terrain-content risk that needs a smaller slice?
- Does this improve correctness at the cost of making the character feel stuck?

### Integrity Risk

- Does this shift trust into the client?
- Does this create new abuse, fraud, or dispute scenarios?
- Does this allow impossible movement, obstacle bypassing, or client-owned navigation truth?

## Preferred Output Format

Provide:

1. recommendation
2. rationale
3. alternatives considered
4. explicit defer list
5. follow-up skill owners
