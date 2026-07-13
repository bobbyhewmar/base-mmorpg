# E2E Docker Online

## Objetivo

Executar a validacao ponta a ponta do fluxo online real pelo browser, usando:

- frontend Vite em `http://localhost:5173`
- proxy `/api`
- backend Go via Docker Compose
- WebSocket real em `ws://localhost:5173/api/v1/gameplay/ws`

## Suite

Arquivos principais:

- `playwright.config.ts`
- `e2e/online-docker.spec.ts`
- `scripts/run-e2e-docker.mjs`

Cobertura atual da suite:

- register
- login
- create character
- `world/enter`
- attach WebSocket
- espera obrigatoria por `region_context`
- `move_intent`
- local click-to-move responsiveness should be added when the responsive hybrid movement refinement ships
- browser reconciliation from pending move preview to authoritative server path
- blocked-destination rejection against server-owned obstacle geodata
- `select_target`
- `use_skill`
- `pick_up_loot`
- `buy_item`
- `sell_item`
- `exchange_item`
- `deposit_item`
- `withdraw_item`
- player-to-player trade offer/accept
- `equip_item`
- `unequip_item`
- validacao do caminho real por proxy `/api`
- validacao do WebSocket pela URL publica do frontend
- validacao de CORS direto falhando fechado para `http://localhost:8080`
- material stack purchase plus shard-based exchange and partial warehouse reconciliation

## Como Rodar

### Instalar dependencias

```bash
npm install
npm run e2e:install
```

### Rodar a suite contra Docker Compose

```bash
npm run e2e:docker
```

O runner faz:

1. `docker compose up -d --build`
2. espera frontend e backend responderem
3. executa `npx playwright test`
4. derruba a stack com `docker compose down`

O runner tambem eleva por env apenas o budget de rate limit de auth/attach dentro da stack de teste para evitar falsos negativos da propria suite, sem alterar os defaults do backend fora do fluxo `e2e:docker`.

### Manter a stack ativa para diagnostico manual

```bash
L2BG_E2E_KEEP_SERVICES=1 npm run e2e:docker
```

No PowerShell:

```powershell
$env:L2BG_E2E_KEEP_SERVICES = "1"
npm run e2e:docker
```

## Diagnostico de Falhas

Quando a suite falhar:

1. revisar o report HTML em `playwright-report/`
2. revisar `test-results/` para traces, screenshots e videos
3. revisar logs do Compose:

```bash
docker compose logs frontend
docker compose logs backend
docker compose logs db-init
docker compose logs postgres
```

4. validar saude basica manual:

```bash
curl http://localhost:5173
curl http://localhost:8080/healthz
docker compose ps
```

## Sinais Esperados

- requests HTTP do browser passam por `http://localhost:5173/api/...`
- requests do browser nao devem ir direto para `http://localhost:8080/v1/...`
- o browser recebe `region_context` antes de montar o mundo online
- o primeiro WebSocket do fluxo online usa `ws://localhost:5173/api/v1/gameplay/ws`
- fetch direto cross-origin para `http://localhost:8080` responde corretamente porque o Compose local publica `L2BG_ALLOWED_ORIGINS` para `http://localhost:5173`
- quando o refinamento responsivo existir, o personagem local deve iniciar movimento antes do retorno autoritativo de pathfinding em latencia normal
- `pick_up_loot` deve funcionar por clique direto no drop e por `pick_up_nearby`, sem retry local, com movimento de aproximacao e delta de inventario vindos do backend

## Limites Intencionais

- a suite dirige `move_intent`, `select_target` e `pick_up_loot` por um bridge minimo do browser para evitar flakiness de raycast/pixel no canvas
- esse bridge nao cria caminho paralelo de gameplay: ele chama os mesmos metodos do `ClientApp` ja usados pelo runtime online
- a cobertura de geodata/pathfinding continua usando destino puro; o bridge nunca injeta path, waypoints, ou `collision_result`
- o fluxo HTTP, proxy, WebSocket, read model, runtime online e HUD continuam sendo os caminhos reais exercitados
