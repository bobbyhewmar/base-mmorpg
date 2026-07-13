# Compact MMORPG MVP

This repository now contains a browser-playable local MVP built as a small Three.js + TypeScript prototype.

## Run

```bash
npm install
npm run dev
```

Open the local Vite URL shown in the terminal.

Useful commands:

```bash
npm run typecheck
npm test
npm run build
```

## Included

- One compact safe town hub: `Dawn Plaza`
- One adjacent hunting field: `Gloam Field`
- One deeper danger pocket: `Ruin Hollow`
- Elevated three-quarter camera with click-to-move
- Click-to-target combat with:
  - `Crescent Strike` single-target attack
  - `Grave Bloom` slower target-centered AoE with split damage
- Simple enemy AI with aggro, melee, death, and respawn
- Loot drops that appear in the world and can be clicked
- Inventory and equipment HUD panels
- Separate item templates and item instances in the local domain model
- Equipping gear changes player stats and visible character parts
- Town NPC task loop with accept and turn-in flow
- Optional local save/load through `localStorage`

## Intentionally Deferred

- Login, accounts, or online character sessions
- Backend services, databases, migrations, or WebSocket networking
- PvP, party, clan, castle, olympiad, and social systems
- Warehouses, trade, pets, mounts, or taming
- Production observability, transactional email, and infrastructure concerns
- Final authored 3D asset pipeline
