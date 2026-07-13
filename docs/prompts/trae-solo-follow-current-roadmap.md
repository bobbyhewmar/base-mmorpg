# Trae SOLO Prompt: Follow Current Roadmap

Use this prompt when handing the project to an executor chat that should continue the roadmap from the current repository state.

```text
Voce esta trabalhando dentro deste repositorio. Leia primeiro TRAE_SOLO_MASTER_PROMPT.md e trate-o como contrato operacional. Leia tambem docs/backlog/online-slice-now-next-later.md, docs/specs/server-terrain-geodata-pathfinding.md, docs/specs/hud-skills-and-hotbars.md, docs/specs/hud-inventory-and-classic-windows.md, docs/interface-architecture.md e skills/threejs-client-engineer/references/client-implementation-guidelines.md.

Antes de alterar qualquer arquivo, rode git status --short e inspecione o estado real do codigo com rg. Nao assuma que uma fase esta pendente so porque o texto antigo dizia isso; compare docs, testes e implementacao real.

Prioridade 1: auditar e fechar a responsividade de movimento.
- Verifique se terrain click no fluxo online faz o player local comecar a andar imediatamente, sem aguardar ack/delta/pathfinding.
- Se ainda houver pausa perceptivel, implemente predicao local reversivel, prediction leash e blend suave para a rota autoritativa.
- Preserve move_intent point-based: o client envia apenas destino; rejeite path, waypoints, collision_result, navigation_result, navmesh_id e geodata_override vindos do client.
- Preserve geodata/pathfinding server-owned, reason codes movement.*, command_seq, dedup e aplicacao serializada de estado.
- Nao persista waypoint/frame movement no Postgres.
- Adicione/ajuste testes unitarios, read-model/runtime/client e E2E se necessario para provar ausencia de pausa visivel, contorno de obstaculo e destino inalcancavel.

Prioridade 2: se a auditoria provar que movimento responsivo ja esta fechado, siga para Fase J - quests e NPC services.
- Quest truth no backend, dialogo como apresentacao client-side.
- Persistir quest state, evitar reward duplicado, reason codes quest.*, NPC services autoritativos e HUD tracker somente quando houver quest real.

Prioridade 3: depois disso, implemente `use_item` autoritativo para consumiveis na shortcut/action bar.
- Preserve o contrato ja entregue de `set_hotbar_state`: ele persiste `skill`, `item`, `action`, slots vazios e `open_bar_count` em `character_hotbar_loadouts`.
- Nao trate a hotbar como skill-only; ela e shortcut/action bar multitarefa.
- Mantenha slots 32x32, visual classico, Active/Passive tabs, navegacao ALT+T/K/C/N/U no topo da janela do personagem, inventory ALT+V fechado por padrao e sem task tracker default.
- Nao recrie a antiga fileira duplicada de skills acima de Active/Passive; esse espaco e exclusivo para alternar Status, Skills, Actions, Clan e Quests.
- Preserve o ghost visual da skill ou item no cursor durante drag para hotbar.

Para cada fatia:
1. implemente uma mudanca vertical pequena e completa;
2. atualize docs/specs/skills se mudar contrato, arquitetura, UI, dados ou fluxo;
3. rode obrigatoriamente via Docker Compose quando aplicavel: docker compose up --build -d frontend, docker compose exec -T frontend npm run typecheck, docker compose exec -T frontend npm test, docker compose exec -T frontend npm run build, docker compose config --quiet;
4. rode backend/go tests quando tocar backend: go test ./... no contexto correto ou via container se o projeto exigir;
5. corrija falhas antes de seguir;
6. pare para pedir input apenas se houver decisao criativa nao documentada, credencial externa, risco legal/IP, conflito entre docs ou dependencia externa indisponivel.

Nao reintroduza UI generica: HUD classica, janelas quadradas, barras azuis, slots 32x32, icon-only grids, tooltips, navegacao ALT+T/K/C/N/U, shortcut/action bar multitarefa e drag ghost de skill/item sao canonicos.
```
