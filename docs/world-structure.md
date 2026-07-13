# World Structure

## Direction

Build a compact MMORPG world, not a giant open world.

The world should feel handcrafted, dense, and easy to read. The target shape is a city-hub structure with short surrounding territories, closer to a classic compact MMORPG loop than to endless traversal.

## Core World Pattern

Each major city acts as a hub with:

- a central safe region
- a short ring of service and social space
- a few adjacent hunting regions
- one or more deeper field or dungeon-adjacent regions

The distance from a city center to meaningful combat space should stay short enough that returning to town never feels like a long commute.

## Region Types

### City Core

- safe zone
- player congregation
- merchants, storage, crafting, repair, and social services
- strong landmark identity and easy orientation

### City Outskirts

- transitional buffer between safety and danger
- low-pressure encounters or tutorial pacing
- visual connection back to the city hub

### Hunting Fields

- primary grind and progression spaces
- dense local loops
- clear pathing and enemy grouping
- strong readability for entrances, exits, and danger escalation

### Deep Regions

- tougher enemy groups
- sharper risk-reward profile
- optional dungeon or boss access later

## Layout Rules

- Prefer smaller contiguous regions over huge empty travel spans.
- Keep every region identity strong enough that players remember it by shape, color, or landmark.
- Keep transitions explicit and legible.
- Avoid overly long dead travel corridors.
- Keep routes short enough that city-to-field and field-to-city loops support repeated play.
- Treat major rocks, ruins, walls, fences, bridges, gates, cliffs, and props as gameplay blockers when they affect traversal.
- Leave readable openings around blockers so automatic alternate route generation feels intentional instead of random.

## City Pattern

Treat each city as a gameplay loop anchor, not just a decoration.

Each city should ideally provide:

- one readable central plaza or gathering point
- one merchant or upgrade cluster
- one storage or utility cluster
- one or more exits to surrounding territories
- one fast mental map for first-time players

## Progression Through Space

The world should communicate progression spatially:

- safer and more social in the center
- more dangerous as players move outward
- stronger monster density or difficulty in deeper regions
- occasional return-to-town loops for reset, trade, and social rhythm

## Terrain and Geodata Rules

Every playable region should have a server-owned terrain/geodata definition.

The geodata should identify:

- navigable surfaces
- static blocking obstacles
- region bounds
- exits or portals to neighboring regions
- safe, combat, restricted, or special traversal areas when needed
- movement-profile constraints such as actor radius or mount access

Art placement and gameplay navigation must stay aligned. If an object looks like a wall, cliff, rock, ruin, or blocked gate, the server geodata should treat it as a blocker. If a route looks open, the server should generally be able to find a path through it.

The first implementation should favor region-scoped geodata over seamless-world complexity.

## Session Design Implications

Support short and medium play sessions:

- a player should be able to log in, reach combat, and make progress quickly
- returning to town should feel deliberate, not tedious
- compact traversal should help keep solo and party loops dense

## Technical Implications

- favor region-scoped interest management
- favor region-scoped geodata and pathfinding
- favor bounded local entity counts
- favor clean handoff between neighboring regions
- avoid architecture that assumes one huge seamless world from day one

## Prototype Slice Recommendation

The first meaningful vertical slice should include:

1. one city hub
2. one adjacent low-level hunting region
3. one slightly deeper region
4. NPC services in town
5. combat, loot, and return-to-town loop
