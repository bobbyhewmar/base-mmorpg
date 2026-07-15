# Trae SOLO Prompt: Follow Current Roadmap

Use this prompt when handing the project to an executor chat that should continue from the current repository state.

```text
Voce esta trabalhando dentro deste repositorio. Leia primeiro TRAE_SOLO_MASTER_PROMPT.md e trate-o como contrato operacional. Leia tambem docs/backlog/online-slice-now-next-later.md, docs/online-readiness-checklist.md, docs/specs/server-terrain-geodata-pathfinding.md, docs/specs/region-presence-known-set.md, docs/specs/hud-skills-and-hotbars.md, docs/specs/hud-inventory-and-classic-windows.md, docs/specs/character-creation-contract.md, docs/interface-architecture.md, docs/world-structure.md e skills/threejs-client-engineer/references/client-implementation-guidelines.md.

Antes de alterar qualquer arquivo:
1. rode git status --short;
2. inspecione docs, testes e implementacao real com rg;
3. nao assuma que uma fase esta pendente so porque um texto antigo dizia isso;
4. preserve mudancas existentes de outros chats e nunca reverta trabalho alheio sem pedido explicito.

Estado atual que voce deve tratar como entregue, salvo evidencia contraria no codigo:
- auth/sessao, command lifecycle, durable command dedup, E2E online, observabilidade minima, progressao duravel, multiplayer presence, class skills/hotbar, inventario/economia, terrain/geodata/pathfinding, quests/NPC services, use_item, pets/taming/mounts, party, social chat, shared XP minimo e party-owned loot minimo ja existem em slices autoritativos.
- A regiao ativa atual usa `stonecross_plaza` apenas como id compativel, mas o mapa oficial foi resetado para uma area limpa 1024x1024 com `clean_plain_1024_geo_v1`, bounds `x=-512..512` e `z=-512..512`.
- Renderer, visual ground, invisible ground raycast/picking plane, server geodata bounds, spawn/checkpoint, exits futuros, mob/NPC/loot placement futuro e testes devem compartilhar o mesmo contrato de mapa.
- Nao reintroduza clamp hardcoded do mapa antigo no client. Nao reaproveite bounds antigos de `dawn_plaza`. `dawn_plaza` pode existir apenas como alias temporario para a regiao limpa 1024x1024.
- Nao reintroduza Stonecross visual, cidade antiga, props antigos, blockers antigos ou spawns iniciais como fallback. O mapa ativo nao possui mobs, NPCs, construcoes, ruas, agua, overlays de terreno, grind zones ou obstaculos autoritativos.
- Character creation inicia com a primeira opcao canonica de cada seletor marcada por padrao. O jogador deve poder digitar apenas o nome e criar; nome duplicado/reservado e rejeitado pelo backend.
- HUD canonica e classica: janelas quadradas, barras azuis, slots 32x32, icon-only grids, tooltips, navegacao ALT+T/K/C/N/U, inventory ALT+V fechado por padrao, shortcut/action bar multitarefa e drag ghost de skill/item/action.
- Sem fallback silencioso. O client apresenta intencao e feedback reversivel; autoridade de gameplay, economia, target, range, movement, pickup, combat, party, chat e pet fica no backend.

Prioridade 1: estabilizar a fundacao recem-entregue antes de adicionar sistemas grandes.
- Se tocar mapa, geodata, movement ou picking, atualize renderer/picking/backend/tests juntos.
- Garanta que qualquer novo mapa nasca com contrato explicito de bounds, geodata_version, spawn/checkpoint, exits, picking plane e testes de borda.
- Preserve movimento hibrido: locomocao local imediata, reconciliation server-owned, command_seq/dedup e sem waypoints enviados pelo client.
- Corrija bugs reais encontrados em gameplay sem criar fallback local que mascare falha de autoridade.

Prioridade 2: avancar social core restante de forma canonica.
- Base de clan e alliance vem depois do party/chat atuais, mantendo backend como autoridade.
- Nao reimplemente party, chat, shared XP ou party loot se ja estiverem cobertos; apenas harden, testar melhor ou expandir quando o codigo mostrar lacuna real.
- Proximas expansoes sociais naturais: clan base, clan chat, invite/join/leave clan, roles simples, auditabilidade e HUD classica minima.

Prioridade 3: avancar para PvP/PK somente depois de social core estar estavel.
- Reuse runtime autoritativo, known-set, combat legality, command lifecycle, reason codes e audit trails.
- Nao pular para siege/olympiad antes de PvP/PK e clan base.

Prioridade 4: aprofundar operacao anti-abuse e investigacao.
- Correlacione economy, trade, chat, party, combat e loot usando os audit trails existentes.
- Nao crie stack nova como Redis, fila ou search sem necessidade medida.

Para cada fatia:
1. implemente uma mudanca vertical pequena e completa;
2. atualize docs/specs/skills se mudar contrato, arquitetura, UI, dados ou fluxo;
3. rode obrigatoriamente via Docker Compose quando aplicavel: docker compose up -d --build, docker compose exec -T frontend npm run typecheck, docker compose exec -T frontend npm test -- --run, docker compose exec -T frontend npm run build, docker compose exec -T backend go test ./..., docker compose exec -T backend go build ./cmd/server, docker compose config --quiet;
4. corrija falhas antes de seguir;
5. pare para pedir input apenas se houver decisao criativa nao documentada, credencial externa, risco legal/IP, conflito entre docs ou dependencia externa indisponivel.
```
