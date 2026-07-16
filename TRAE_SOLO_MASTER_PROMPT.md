# TRAE SOLO MASTER PROMPT

Este arquivo e a fonte de verdade operacional para um chat executor trabalhar neste projeto ate a finalizacao do jogo, sem depender de ping-pong constante com um chat validador.

Use este documento para:

- entender tudo que ja foi decidido
- entender tudo que ja existe no repositorio
- entender o que ainda falta construir
- escolher a proxima fatia de trabalho sem pedir permissao desnecessaria
- implementar, testar, corrigir e documentar de forma autonoma
- parar apenas quando houver uma decisao realmente nao mapeada ou uma dependencia externa impossivel de resolver localmente

## Prompt curto para iniciar o Trae SOLO

Cole este prompt no chat executor:

```text
Voce esta trabalhando dentro deste repositorio. Leia primeiro o arquivo TRAE_SOLO_MASTER_PROMPT.md na raiz do projeto e trate ele como contrato operacional do desenvolvimento.

Seu trabalho e executar o projeto ate a finalizacao do jogo, seguindo as decisoes, prioridades, arquitetura, testes, docs e criterios de parada definidos nesse arquivo.

Nao pergunte ao usuario sobre detalhes que ja estao mapeados no master prompt ou nas docs do repositorio. Quando houver ambiguidade tecnica comum, tome a decisao mais simples, testavel, observavel e alinhada com a arquitetura existente.

Trabalhe em fatias verticais completas. Para cada fatia:
1. inspecione o estado real do codigo e das docs antes de alterar;
2. implemente a menor mudanca completa que avance o roadmap;
3. adicione ou atualize testes;
4. rode as validacoes relevantes;
5. corrija falhas ate passar;
6. atualize documentacao e skills quando a decisao mudar arquitetura, regras de jogo, contratos, dados, seguranca, cliente, backend ou operacao;
7. registre claramente o que foi feito, o que foi validado e qual e a proxima fatia.

Voce so deve parar para pedir input do usuario quando:
- a decisao depender de gosto criativo ainda nao definido;
- a decisao exigir credenciais, dominio, provedor pago, segredo, conta externa ou acao fora do ambiente local;
- existir conflito direto entre duas decisoes documentadas;
- houver risco legal/IP/branding;
- a proxima etapa depender de material de referencia que nao esta disponivel no repositorio.

Caso contrario, continue executando o roadmap em ordem de prioridade ate o jogo estar completo.
```

## Identidade do projeto

O projeto e um MMORPG compacto web-first inspirado conceitualmente em Lineage 2 e Mu Online, mas com arquitetura, assets, codigo, dados e regras proprios.

Direcao de produto:

- profundidade de sistemas parecida com Lineage 2
- simplicidade espacial e leitura de mundo parecida com Mu Online
- mundo compacto, nao mundo aberto gigante
- cidades centrais com territorios curtos ao redor
- a regiao jogavel oficial atual ainda usa o `region_id` `stonecross_plaza` por compatibilidade, mas o conteudo de mapa foi resetado para uma area limpa de prototipo 1024x1024 sem cidade, mobs, NPCs, construcoes, props, ruas, agua, overlays de terreno ou spawns iniciais
- regioes jogaveis novas devem nascer com area padrao 1024x1024 em coordenadas de mundo, normalmente bounds `x=-512..512` e `z=-512..512`, salvo decisao explicita documentada
- nao criar cidade nova apenas como visual em cima de bounds antigos; toda expansao de mapa precisa atualizar renderer, ground raycast, spawn/checkpoint, geodata autoritativa e testes
- geodata atual e `clean_plain_1024_geo_v1`: area jogavel 1024x1024, bounds `x=-512..512` e `z=-512..512`, sem obstaculos autoritativos; todo o mapa e caminhavel ate novo conceito ser definido
- renderer, ground raycast/picking plane, server geodata bounds, spawn/checkpoint e testes precisam usar a mesma area canônica; e proibido clamp hardcoded de mapa antigo no client
- assets runtime do client devem ficar em `src/assets` e ser resolvidos pelo Vite via imports/catalogos de modulo; nao colocar novos assets runtime em `public/assets`
- assets de mapa ja publicados em `src/assets/maps` ficam preservados como biblioteca de conteudo; eles nao fazem parte do mapa ativo enquanto o novo conceito nao for aprovado
- `Medieval Village MegaKit[Standard]` esta disponivel como fonte em `3DAssets/Medieval Village MegaKit[Standard]` e e o kit principal planejado para futuras vilas/cidades medievais; publicar para `src/assets/maps/...` apenas os modulos realmente usados por uma fatia completa que atualize layout visual, picking bounds, geodata, spawn/checkpoint, blockers e testes
- nao reintroduzir Stonecross visual, mapa antigo, props antigos, blockers antigos ou spawns antigos como fallback; se `dawn_plaza` aparecer, deve ser apenas alias temporario para a mesma geodata limpa 1024x1024
- escala de mapa e construcoes deve favorecer leitura Mu Online-like, sem repetir a proporcao monumental de Lineage 2 para cada predio
- escala visual de players, outros players e NPCs e decisao do renderer; a escala atual de personagens foi reduzida novamente para 50% da referencia visual anterior para combinar melhor com estruturas/mapa compacto; mobs, companions, mounts, ranges, geodata, pathfinding e locomoção autoritativa nao mudam por ajuste visual de personagem sem pedido explicito e validacao separada
- excecao atual ja decidida pelo usuario: a escala visual menor exigiu recalibrar camera e velocidade autoritativa; camera atual usa offset `(-7.5, 9, 7.5)`, zoom minimo `3.8`, zoom maximo `28`, guard rail de chao `2.4`; velocidades canonicas atuais sao `Fighter=3.225`, `Mage=3.075`, botas `pathrunner=0.15`, `whisperstep=0.13`, `ruinbound=0.225`, montaria `mireling_strider=4.05`
- o client deve prever movimento usando `stats.move_speed` autoritativo quando disponivel e so recorrer ao template local antes do primeiro snapshot; backend e frontend nao podem divergir nesse valor para evitar snaps/teleportes e falsos bloqueios de velocidade
- loot visual deve ser pequeno e proximo do chao; preserve hitbox invisivel maior para clique/coleta sem aumentar a representacao visivel do drop
- estruturas modulares GLB devem preservar proporcao original do asset e ser repetidas como pecas; nao estique uma parede, telhado ou estrutura em escala nao-uniforme para formar uma casa inteira
- assets retro e medievais de mapa devem usar apenas escala escalar/uniforme; se uma casa, muro, portao, doca, escada ou cerca precisar ocupar mais area, repita modulos em vez de deformar o GLB por eixo
- novos mobs/NPCs/spawns devem entrar apenas por sistema de spawn backend-owned e persistido ou por um novo mapa aprovado; nao semear mobs/NPCs diretamente no estado inicial da regiao limpa
- mobs possuem personalidade canonica: `passive` nao inicia combate por proximidade e so entra em `aggro` apos sofrer dano; `aggressive` detecta o personagem dentro do raio de aggro, persegue sem hesitar e ataca quando alcanca o range
- movimento de mobs continua autoritativo no backend, mas a cena Three.js deve interpolar a posicao visual do procedural ate o snapshot mais recente para evitar micro-teleportes; nao mover autoridade para o client
- mobs procedurais devem ter animacao simples de idle/walk no corpo, cabeca e pernas para perseguicao parecer caminhada, nao deslizamento
- camera elevada de leitura clara, lembrando Mu Online na proporcao personagem/terreno
- camera orbita sempre em torno do personagem como pivô visual
- botao direito do mouse nunca seleciona target, nunca move e nunca abre menu de contexto do navegador dentro do mundo
- arrastar com botao direito rotaciona a camera horizontalmente e verticalmente ao redor do personagem, sempre mantendo o personagem como pivo
- camera vertical pode chegar perto do nivel do chao, mas nunca deve atravessar o terreno; ao colidir com a protecao do chao, a camera encurta a orbita e chega mais perto do personagem em vez de passar por baixo do mapa
- o alvo de look-at/follow da camera deve ser suavizado e independente do bob/animação do mesh do personagem; correcao autoritativa, desnivel ou animacao de corrida nao podem fazer a camera tremer
- scroll do mouse aproxima e afasta a camera dentro de limites controlados
- combate click-to-target ao estilo Lineage 2, nao combate de spot como Mu Online
- a aproximacao automatica de skill/ataque basico deve navegar ate um ponto caminhavel dentro do range da acao, nunca ate o centro do alvo
- starter gear e class-specific: a criacao atual expõe apenas `Human` com `Fighter` e `Mage`; `Fighter` usa pack fisico e `Mage` usa pack mistico; outras classes entram apenas quando o catalogo autoritativo e os tipos runtime forem expandidos
- visuais de player e outros players devem usar `Universal Base Characters` como corpo canonico padrao em `src/assets/characters/universal-base`; as classes usam `gltf_base_character`, variantes `Male`/`Female`, cabelos rigged por sexo, `hair_color` persistido como `#RRGGBB`, tres `skin_type` por sexo e clips da Universal Animation Library; assets legados de personagem nao sao a fonte ativa de personagem
- `hair_style` continua persistido/catalogado e deve mapear somente cabelos sex-compatible do catalogo de assets; `hair_color` deve tingir apenas materiais/meshes de cabelo e nunca pele/corpo/equipamento; nao adicionar `Body Type` enquanto existir apenas um corpo real por sexo
- `Modular Character Outfits - Fantasy` deve permanecer disponivel como biblioteca de equipamento visual futuro; outfit nao e corpo base de classe, outfit representa item equipado quando a camada autoritativa de visual de equipamento for implementada
- nao renderize humanoide procedural como fallback enquanto o asset canonico carrega; deixe o personagem invisivel/transparente ate o asset configurado estar pronto, e trate asset ausente como erro de conteudo explicito
- criacao de personagem inicia com o primeiro template canonico do catalogo autoritativo ja selecionado: `Human`, `Fighter`/`Mage`, `Male`/`Female`, `hair_style`, `hair_color` default do catalogo e `skin_type`; `sex` escolhe o unico corpo real disponivel para aquele sexo, e o jogador pode digitar apenas o nome e criar, enquanto nome reservado/duplicado continua sendo rejeitado pelo backend
- personagens 3D e ambiente 3D em Three.js
- HUD em HTML/CSS, separada da cena 3D
- backend autoritativo em Go
- Postgres como fonte primaria de verdade duravel
- Linux como ambiente de producao
- arquitetura simples, testavel, observavel e sem overengineering

O projeto nao e mais um board game digital. A pasta ainda se chama `L2 BOARD GAME`, mas a direcao atual e MMORPG compacto.

## Principios inegociaveis

### Fonte conceitual, nao copia literal

As fontes de Lineage 2 ou outras bases estudadas servem para extrair conceitos, fluxos, responsabilidades e prioridades.

Referencia padrao de estudo para features Lineage-like:

- `D:\Jogos\Lineage II\Servidores\Lucera\Souce\main`

Antes de implementar uma feature que exista ou tenha equivalente claro em Lineage 2, o executor deve analisar essa source Lucera como referencia direta de comportamento, ciclo de vida, responsabilidades, dados envolvidos, validacoes, restricoes e casos de borda. Depois da analise, deve transformar o conceito em um design proprio deste projeto, alinhado a Go backend autoritativo, Postgres como verdade duravel, Three.js/HTML client, command lifecycle replay-safe, testes e observabilidade.

Nunca portar literalmente:

- packet shape legado
- ids especificos
- schema legado
- acoplamento de enum antigo
- hacks de loader
- nomes de customizacao
- caminhos locais de source
- quirks de branch

Sempre transformar o conceito em documentacao, contratos e implementacao propria deste projeto.

### Autoridade do backend

O client nunca e autoridade para:

- identidade
- sessao
- posse de personagem
- criacao de personagem
- movimento final
- colisao, geodata e pathfinding
- rota e waypoints autoritativos
- target legal
- dano
- cooldown
- HP/MP/XP
- loot
- inventario
- equipamento
- economia
- preco, imposto, desconto ou total de compra
- progressao
- PvP/PK
- siege/olympiad
- pets/mounts

O client envia intencao. O backend valida e aplica.

### Sem cultura de fallback

Nao criar fallback silencioso que mascare erro de autoridade.

Proibido:

- online falhar e cair automaticamente para regras locais
- cache virar verdade de gameplay
- Redis virar fonte de verdade de progressao/economia
- client recalcular resultado autoritativo
- client resolver path/colisao e backend aceitar como verdade
- retry escondido em comando nao-idempotente
- degradar seguranca para preservar conveniencia
- client inventar opcoes canonicas de catalogo, classe, raca, skill, item, regiao ou regra quando a fonte autoritativa nao retornar dados
- client inventar ou substituir aparencia canonica de personagem quando `race`, `sex`, `base_class`, `hair_style`, `hair_color` ou `skin_type` nao vierem da criacao persistida ou da presenca autoritativa
- substituir dado/asset canonico ausente por outro dado "parecido" para manter a tela funcionando

Quando algo falha, rejeitar explicitamente com reason code estavel e UI clara.

### Simples antes de distribuido

Padrao tecnico aceito:

- Go modular monolith
- PostgreSQL
- WebSocket para gameplay online
- HTTP para auth/bootstrap
- Three.js no mundo 3D
- HTML/CSS para HUD
- Docker Compose para desenvolvimento local
- Postgres job tables antes de fila dedicada
- Redis so depois de necessidade medida
- Elasticsearch/OpenSearch so depois de necessidade real de busca
- Scylla/Cassandra nao usar para o core atual
- microservices fora de escopo inicial

### Observavel antes de escalar

Antes de adicionar infraestrutura complexa, medir:

- request rate
- latencia p95/p99
- erro por endpoint
- command validation failures por reason code
- WebSockets ativos
- sessoes ativas
- personagens ativos
- ocupacao por regiao
- comandos de combate/loot/movement por segundo
- tempo e falhas de pathfinding por regiao
- pressao do pool Postgres
- jobs pendentes e idade dos jobs
- falhas de login/register/attach
- tentativas suspeitas

## Estado atual do repositorio

### Raiz

Arquivos principais existentes:

- `package.json` - app frontend Vite/TypeScript/Three.js e scripts npm
- `package-lock.json` - lockfile Node
- `index.html` - entrada web
- `vite.config.ts` - Vite com proxy opcional `/api` para backend
- `tsconfig.json` - TypeScript
- `README.md` - resumo do MVP local
- `docker-compose.yml` - frontend, backend, Postgres e db-init
- `Dockerfile.frontend` - imagem de dev do frontend
- `.env.example` - variaveis locais
- `.dockerignore`
- `.gitignore`
- `TRAE_SOLO_MASTER_PROMPT.md` - este arquivo

Observacao importante:

- o repositorio pode estar sem commits iniciais e com muitos arquivos untracked
- sempre rodar `git status --short` antes de alterar
- nao reverter mudancas de usuario

### Frontend local e online

Arquivos principais:

- `src/main.ts`
- `src/app/clientApp.ts`
- `src/app/preGameMachine.ts`
- `src/app/authFlow.ts`
- `src/app/characterCreationOptions.ts`
- `src/runtime/worldRuntime.ts`
- `src/online/contracts.ts`
- `src/online/client.ts`
- `src/online/readModel.ts`
- `src/game/domain/types.ts`
- `src/game/domain/game.ts`
- `src/game/data/templates.ts`
- `src/game/scene/scene3d.ts`
- `src/game/platform/localSave.ts`
- `src/ui/hud.ts`
- `src/styles.css`

O frontend ja possui:

- modo local explicito
- modo online explicito
- login como primeira tela do jogador, sem escolha inicial entre online/local
- fundo de floresta dark fantasy inspirado no clima de Interlude, sem logos ou branding de terceiros
- login/register online
- login/register usando componentes visuais compartilhados e botoes canonicos do jogo
- fluxo pre-game canonico: login, EULA classico, selecao de servidor/mundo, selecao/criacao de personagem e so entao entrada no mundo
- hook visual de verificacao pendente
- hook visual de recovery
- listagem de personagens
- catalogo autoritativo de criacao
- criacao de personagem
- fluxo de criacao com `race`, `base_class`, `sex`, `hair_style`, `hair_color`, `skin_type` e `name` como escolhas canonicas persistidas
- preview de criacao e lobby/mundo 3D lendo a aparencia escolhida, sem descartar selecoes ao entrar no jogo
- `world/enter`
- attach WebSocket
- espera por `region_context` antes de montar mundo online
- read model online com sequencia de comandos
- bloqueio por desync de revision/region_revision
- comandos online para movement, target, skill, loot, `interact_npc`, `use_item`, equip, unequip e `set_hotbar_state`
- runtime compartilhado local/online
- HUD hibrida em HTML/CSS
- player frame compacto com level, nome, CP/HP/MP e valores sobre as barras
- target frame no topo quando mob, NPC ou player esta selecionado
- chat/system log no canto inferior esquerdo
- shortcut/action bar classica bottom-center com slots `32x32px`, uma barra azul de drag na esquerda, botao `16x16px` de expandir na direita e linhas crescendo de baixo para cima
- inventario `ALT+V` fechado por padrao, com janela classica, grid de icones `32x32px`, tooltip de item, footer de moeda/peso e botao de fechar funcional
- mini menu fixo no canto inferior direito, identico ao estilo classico, com uma barra azul vertical de drag na esquerda, quatro botoes `32x32px` para Status, Inventory, Map e System, tooltips `Character Status (Alt+T)`, `Inventory (Alt+V)`, `Map (Alt+M)` e `System (Alt+X)`, mapa placeholder, janela System classica e confirmacao `Do you wish to exit the game?` com `OK` e `Cancel`
- familia de janelas do personagem com `ALT+T` Status, `ALT+K` Skills, `ALT+C` Actions, `ALT+N` Clan e `ALT+U` Quests
- topo das janelas do personagem reservado para botoes de navegacao `32x32px`, nao para duplicar skills
- skill book `ALT+K` com janela classica, abas `Active`/`Passive`, grid de icones `32x32px`, tooltips e drag persistente de skills ativas para hotbar via `set_hotbar_state`
- itens do inventario podem ser arrastados para a shortcut/action bar como binding persistente via `set_hotbar_state`; equipaveis chamam o mesmo fluxo de equip ao clicar
- actions do `ALT+C`, como `basic_attack`, `pick_up_nearby`, `party_invite` e `party_leave`, podem ser arrastadas para a shortcut/action bar como binding persistente via `set_hotbar_state`
- `pick_up_nearby` e clique direto em drop enviam um unico `pick_up_loot`; nao existe retry local no client, o backend resolve aproximacao, range, persistencia e desaparecimento do loot
- drag de skill ou item exibe o icone preso ao cursor ate soltar ou cancelar
- consumiveis usam comando autoritativo `use_item` no inventario e na shortcut/action bar
- cena Three.js com mundo compacto, mobs, NPC, loot, selecao, movimento e feedback visual
- localStorage apenas para prototipo local

### Backend Go

Arquivos principais:

- `backend/go.mod`
- `backend/go.sum`
- `backend/Dockerfile`
- `backend/cmd/server/main.go`
- `backend/internal/app/server.go`
- `backend/internal/app/gameplay_runtime.go`
- `backend/internal/app/models.go`
- `backend/internal/app/store.go`
- `backend/internal/app/store_memory.go`
- `backend/internal/app/store_postgres.go`
- `backend/internal/app/character_stats.go`
- `backend/internal/app/character_items.go`

O backend ja possui:

- HTTP server em Go
- `GET /healthz`
- `POST /v1/auth/register`
- `POST /v1/auth/login`
- `GET /v1/characters`
- `GET /v1/characters/catalog`
- `POST /v1/characters`
- `POST /v1/world/enter`
- `GET /v1/gameplay/ws` como WebSocket
- attach por `attach_session`
- `region_context`
- `ack`
- `reject`
- `delta`
- `entity_appear`
- `entity_disappear`
- `position_correction`
- runtime autoritativo basico
- catalogo de criacao retorna `appearance_options` por raca
- personagens persistem `hair_style`, `hair_color` e `skin_type` em memoria/Postgres
- presenca de jogador em `region_context`, `entity_appear` e deltas carrega raca, classe, sexo e aparencia para renderizacao remota
- expected `command_seq` em memoria
- target por known-set
- skill single target
- skill AoE target-centered com dano dividido
- cooldown runtime
- dano, morte, respawn de mobs
- morte e respawn de player
- loot autoritativo
- pickup atomico em memoria/Postgres
- pickup de loot fora do range imediato enfileira movimento autoritativo ate um ponto caminhavel dentro do raio de coleta e conclui a coleta no tick do runtime
- inventario/equipamento
- stats derivados por equipamento
- fan-out basico de loot para sessoes na mesma regiao
- Postgres quando `L2BG_DATABASE_URL` existe
- memoria quando nao existe `L2BG_DATABASE_URL`

### Banco e operacao local

Arquivos principais:

- `docker-compose.yml`
- `ops/db-init/bootstrap.sql`
- `ops/db-init/run.sh`
- `docs/local-docker-compose.md`
- `.env.example`

Estado atual:

- Compose define `frontend`, `backend`, `postgres`, `db-init`
- Postgres usa imagem `postgres:17-alpine`
- `db-init` aplica schema idempotente
- backend usa `L2BG_DATABASE_URL`
- frontend usa `VITE_L2BG_API_BASE_URL`
- Vite pode proxyar `/api` para backend
- WebSocket publico pode ser configurado por `L2BG_PUBLIC_WS_URL`
- account sessions duraveis usam Postgres quando `L2BG_DATABASE_URL` esta configurado
- CORS HTTP direto e configurado por `L2BG_ALLOWED_ORIGINS`
- TTL de access token e configurado por `L2BG_ACCESS_TOKEN_TTL`

### Docs existentes

Docs principais:

- `docs/architecture-overview.md`
- `docs/engineering-principles.md`
- `docs/domain-and-data.md`
- `docs/dependency-boundaries.md`
- `docs/delivery-roadmap.md`
- `docs/client-runtime-strategy.md`
- `docs/interface-architecture.md`
- `docs/combat-and-targeting.md`
- `docs/world-structure.md`
- `docs/source-material-reference.md`
- `docs/transactional-email-integration.md`
- `docs/runtime-operations.md`
- `docs/local-docker-compose.md`
- `docs/skill-matrix.md`

Specs principais:

- `docs/specs/account-auth-and-character-entry.md`
- `docs/specs/account-auth-http-surface.md`
- `docs/specs/bootstrap-flow.md`
- `docs/specs/character-creation-contract.md`
- `docs/specs/command-envelope.md`
- `docs/specs/command-lifecycle.md`
- `docs/specs/hud-inventory-and-classic-windows.md`
- `docs/specs/hud-skills-and-hotbars.md`
- `docs/specs/region-presence-known-set.md`
- `docs/specs/runtime-state.md`
- `docs/specs/session-ownership-and-cross-instance-presence.md`
- `docs/specs/postgres-gameplay-event-outbox.md`
- `docs/specs/cross-instance-region-player-projections.md`
- `docs/specs/server-terrain-geodata-pathfinding.md`

Backlog:

- `docs/backlog/online-slice-now-next-later.md`

ADR:

- `docs/adr/adr-001-online-slice-authority-boundaries.md`

Estudo conceitual:

- `docs/interlude-source-study/README.md`
- `docs/interlude-source-study/01-character-world-and-lifecycle.md`
- `docs/interlude-source-study/02-combat-stats-skills-and-pve.md`
- `docs/interlude-source-study/03-progression-classes-quests-and-items.md`
- `docs/interlude-source-study/04-social-pvp-and-territorial-systems.md`
- `docs/interlude-source-study/05-implementation-and-scope-guidance.md`
- `docs/interlude-source-study/06-class-template-and-stat-baseline.md`
- `docs/interlude-source-study/07-inventory-equipment-and-item-usage.md`

Atencao:

- `docs/interlude-source-study/README.md` ainda menciona caminho local absoluto da source estudada
- remover caminhos locais especificos de documentacao publica
- trocar por "fonte estudada" ou "source tree analisada"

Prompts de extracao:

- `docs/prompts/trae-solo-lineage2-class-stat-extraction.md`
- `docs/prompts/trae-solo-lineage2-inventory-equipment-item-usage.md`
- `docs/prompts/trae-solo-lineage2-skills-formulas-class-transfer-subclass.md`
- `docs/prompts/trae-solo-build-playable-mvp.md`

Regra:

- prompts de extracao devem sempre dizer para extrair conceito e transformar em documentacao propria
- usar `D:\Jogos\Lineage II\Servidores\Lucera\Souce\main` como source root padrao de estudo quando o executor tiver acesso local a essa pasta
- nunca transformar esse source root em dependencia runtime, caminho hardcoded da aplicacao, import, copia de codigo, schema legado ou contrato de dados do nosso jogo
- nao rotular nada como custom/retail; tratar como definicao da fonte estudada

### Skills existentes

Skills do projeto:

- `skills/product-game-director`
- `skills/solution-architect`
- `skills/systems-designer`
- `skills/threejs-client-engineer`
- `skills/backend-gameplay-engineer`
- `skills/data-persistence-engineer`
- `skills/appsec-game-integrity-engineer`
- `skills/devops-infra-engineer`
- `skills/qa-automation-engineer`

Usar essas personas como lentes de revisao. Se houver subagentes disponiveis, delegar por skill. Se nao houver, emular a revisao de cada papel antes de fechar uma fatia.

Responsabilidades:

- `product-game-director` - preserva visao do jogo, prioridades e cortes de escopo
- `solution-architect` - fronteiras, contratos, simplicidade, escalabilidade medida
- `systems-designer` - progressao, combate, classes, pets, PvP, economia, loops
- `threejs-client-engineer` - cena, camera, input, HUD, read model, UX
- `backend-gameplay-engineer` - comandos autoritativos, runtime, combat, loot, sessoes
- `data-persistence-engineer` - Postgres, transacoes, indices, migrations, schemas
- `appsec-game-integrity-engineer` - auth, trust boundaries, abuso, anti-cheat
- `devops-infra-engineer` - Docker, Linux, observabilidade, deploy, runbooks
- `qa-automation-engineer` - testes, E2E, load tests, regressao

### Assets

Ha referencias em:

- `art/characters/concepts_lowpoly`
- `art/characters/turnarounds_tpose`

Direcao:

- lowpoly
- dark fantasy
- misterioso
- sem textura hiper-realista
- silhuetas fortes
- equipamentos mudam visual
- proporcao personagem/terreno clara como Mu Online

## Validacoes atuais conhecidas

Ultima validacao manual conhecida:

```bash
docker compose up -d --build
docker compose exec -T frontend npm run typecheck
docker compose exec -T frontend npm test -- --run
docker compose exec -T frontend npm run build
docker compose exec -T backend go test ./...
docker compose exec -T backend go build ./cmd/server
docker compose config --quiet
git diff --check
```

Resultado conhecido:

- frontend: suite passando via container
- build frontend: passando
- backend: passando
- compose config: valido
- aviso: bundle JS grande por Three.js, esperado nesta fase

Antes de qualquer fatia nova:

```bash
git status --short
docker compose up -d --build
docker compose exec -T frontend npm run typecheck
docker compose exec -T frontend npm test -- --run
docker compose exec -T frontend npm run build
docker compose exec -T backend go test ./...
docker compose exec -T backend go build ./cmd/server
docker compose config --quiet
git diff --check
```

Se uma validacao falhar, corrigir antes de seguir.

## Decisoes de game design ja mapeadas

### Tipo de jogo

O jogo e MMORPG compacto.

Nao e:

- mundo aberto gigante
- arena isolada
- board game digital
- idle game
- clone de Mu Online
- clone literal de Lineage 2

Formula:

- visual e leitura espacial compacta de Mu Online
- profundidade de sistemas de Lineage 2
- arquitetura e conteudo proprios

### Camera

Direcao:

- camera elevada
- leitura diagonal/isometrica ou tres-quartos
- proporcao personagem/terreno lembrando Mu Online
- foco em clareza de alvos e regioes curtas
- camera nao deve atrapalhar target selection

Evitar:

- camera livre demais na fase inicial
- zoom que quebre leitura de combate
- camera cinematica que prejudique jogabilidade

### Mundo

Estrutura:

- cidades como hubs sociais e de servicos
- cada cidade tem centro claro
- territorios curtos ao redor
- regioes com risco legivel
- regioes densas, nao vazias
- landmarks fortes
- caminhos e retornos claros

Primeiro loop:

- cidade segura
- estrada/portao
- campo de caca
- bolso de perigo/ruinas

### Arte

Direcao:

- dark fantasy
- misterioso
- lowpoly
- legivel
- atmosfera por luz, silhueta, fog, ruinas, paleta e composicao
- equipamentos visualmente relevantes

Evitar:

- textura detalhada demais
- ruido visual
- cartoon brilhante demais
- UI generica roxa
- personagem sem mudanca visual por equipamento

### Movimento

Modelo:

- target point
- clicar no terreno para mover
- backend valida movimento online contra terrain/geodata server-side
- client envia apenas destino, nunca path/waypoints/resultado de colisao
- client deve iniciar movimento visual preditivo imediatamente apos o click/dispatch
- client pode usar steering/collision helpers locais apenas como apresentacao reversivel
- client deve reconciliar com rota autoritativa quando o backend responder
- personagem local nao deve ficar parado aguardando pathfinding do backend em latencia normal
- usar prediction leash para evitar que o client corra longe demais antes da autoridade
- pathfinder do backend deve gerar rotas alternativas para contornar pedras, paredes, ruinas, cercas, penhascos e obstaculos similares
- pathfinding deve ter budget/cancelamento e nao travar loop de socket/regiao
- goroutines/worker pool podem ser usados, mas sem criar corrida de estado, outcome fora de ordem ou goroutine sem limite
- destino bloqueado ou inalcancavel deve gerar reject/correction com reason code estavel
- Postgres nao persiste cada frame
- posicao e checkpointada em fronteiras duraveis

### Target e combate

Modelo principal:

- clique no mob para selecionar target
- skill hostil exige target valido
- depois de selecionar target, skill pode ser usada
- tab-cycle pode existir como secundario, mas nao substitui a regra de target valido
- nao usar modelo Mu Online onde skill bate em area sem target e sem decisao

Skills:

- single target rapido
- area skills existem, mas sao mais lentas
- AoE deve ter dano dividido ou budget dividido
- AoE deve ser target-centered ou outra forma explicita validada pelo backend
- area preview visual deve diferenciar movimento, target e skill area

### Classes e stats

Direcao:

- stats de classe devem seguir conceitualmente os baselines extraidos de Lineage 2
- tratar class templates como referencia canonica conceitual
- transformar em schema e regras proprias
- suportar `Fighter` e `Mage` como as duas classes criaveis iniciais para `Human`; novas classes, transfers e subclasses entram apenas quando o catalogo autoritativo e os tipos runtime forem expandidos
- expandir para classes, transfers e subclass depois
- progresso pode ser infinito ou ate 65565, conforme decisao do usuario

Ainda precisa detalhar:

- curva final de XP
- limites de atributos por nivel alto
- scaling de HP/MP/CP
- progressao de skill learning
- classe final por raca
- subclass e class transfer

### Leveling

Direcao desejada:

- leveling muito longo
- possivelmente infinito ou limite 65565
- nao implementar sem curva matematica e impacto de balanceamento
- preservar simplicidade de leitura apesar de progressao profunda

### Itens e equipamento

Regras:

- item template imutavel
- item instance mutavel
- container membership e equip slot sao a verdade de localizacao
- equipamento muda stats
- equipamento muda visual
- preco e valor sempre derivados no backend
- inventario/equipment devem ser persistentes

Slots minimos atuais:

- weapon
- chest

Slots futuros:

- helm
- gloves
- boots
- legs
- necklace
- earrings
- rings
- belt/cloak quando fizer sentido
- pet/mount equipment depois

### Pets, montarias e domar monstros

Direcao:

- alguns monstros podem ser domados
- monstro domado vira pet, companion ou mount
- especie mantem identidade visual
- pet/mount e ator autoritativo
- ownership persistente
- summon/unsummon/mount/dismount sao comandos autoritativos
- mounted state pode alterar movimento e restricoes de skill

Ainda falta construir:

- taming profiles
- companion templates
- companion instances
- active companion slot
- mount profiles
- comandos tame/summon/unsummon/mount/dismount
- HUD de pet/mount
- AI de companion
- regras de morte/recall/revive

### Social e endgame

Sistemas desejados:

- party
- clans
- alliances
- PvP
- PK
- karma/penalidade
- instancias
- castle sieges
- olympiads
- eventos PvP
- quests profundas
- bosses
- economia, vendors, warehouse, trade

Nao implementar tudo antes do core online estar solido.

Ordem recomendada:

1. personagem e mundo
2. movimento e target
3. combate basico
4. loot/inventario/equip
5. classes/stats/skills
6. quest/NPC
7. party
8. PvP/PK
9. clan/alliance
10. instancias
11. siege
12. olympiad
13. eventos/liveops

## Arquitetura congelada

### Backend

Usar:

- Go
- modular monolith
- standard library sempre que possivel
- dependencias pequenas e focadas
- ports/adapters
- Postgres como verdade duravel
- runtime em memoria para estado online momentaneo
- terrain/geodata e pathfinding autoritativos por regiao
- WebSocket para comandos online
- HTTP para bootstrap/pre-game

Nao usar agora:

- microservices
- ScyllaDB
- Cassandra
- Kafka
- Redis como verdade
- ORM pesado que vaze para dominio
- framework que acople handlers ao core

### Banco

PostgreSQL e suficiente para o alvo inicial, incluindo 2 mil jogadores ativos, desde que:

- regioes sejam compactas
- geodata seja versionada por regiao e carregada/cached no runtime
- updates sejam por interesse/known-set
- transacoes sejam curtas
- comandos conflitantes sejam serializados
- ownership online use lease duravel e fencing monotono por personagem
- indexes sejam corretos
- PgBouncer seja usado em producao
- load tests sejam feitos com o mix real

Redis pode entrar depois para:

- presence
- rate limit
- fan-out cross-instance
- cache efemero de leitura

Redis nao pode entrar para:

- progressao
- inventario
- economia
- resultado de combate
- verdade de sessao duravel

Search externo pode entrar depois para:

- busca admin
- item/lore/support fuzzy search
- alto volume de indexacao

Comecar com:

- PostgreSQL full-text search
- queries bem indexadas

### Client

Runtime principal:

- web browser

Desktop/Electron:

- opcional
- apenas adapter/empacotamento
- nao pode virar runtime canonico antes da web estar solida

Three.js:

- mundo 3D
- personagens
- mobs
- NPCs
- loot
- selection rings
- destination markers
- path/geodata debug markers somente quando habilitados explicitamente para desenvolvimento
- blocked movement feedback
- area previews
- floating combat feedback

HTML/CSS:

- HUD
- chat
- inventory
- equipment
- target frame
- player frame
- quest tracker
- hotbars
- skill book
- modals
- tooltips
- character creation
- login/register

### Dependencias substituiveis

Core nao deve conhecer:

- Resend SDK
- framework HTTP
- modelos de provider
- payload bruto de webhook
- ORM-specific annotations
- request/response objects de lib

Criar portas internas:

- EmailSender/NotificationDispatcher
- JobQueue/JobRunner
- AccountRepository
- CharacterRepository
- SessionRepository
- ItemRepository
- MetricsRecorder
- Clock/ID generator quando ajudar testes

Adapters nas bordas:

- HTTP
- WebSocket
- Postgres
- Resend
- logging/metrics
- jobs

## Fluxos ja especificados

### Bootstrap online

Fluxo oficial:

1. cold client abre em login/register
2. player registra ou loga
3. client busca characters
4. client busca character catalog
5. player escolhe ou cria character
6. client chama `POST /v1/world/enter`
7. backend cria gameplay session `pending_attach`
8. client abre WebSocket
9. client envia `attach_session`
10. backend valida attach
11. backend envia `region_context`
12. gameplay command flow comeca

Invariante:

- client nao entra no mundo online sem `region_context`

### Envelope de comando

Formato oficial:

```json
{
  "protocol_version": 1,
  "command_id": "cmd_...",
  "command_seq": 1,
  "client_sent_at_ms": 123456789,
  "type": "use_skill",
  "payload": {
    "skill_id": "crescent_strike",
    "target_id": "mob_1"
  }
}
```

Campos proibidos no comando gameplay:

- account_id
- session_id
- character_id
- actor_id
- region_id
- damage
- hp_after
- mp_after
- cooldown_after
- position_after
- path
- waypoints
- collision_result
- navigation_result
- navmesh_id
- geodata_version
- geodata_override
- known_set
- price
- item_price
- vendor_price
- tax_amount
- discount_amount
- total_cost
- currency_delta

Actor sempre vem de:

```text
connection -> session -> character
```

### Lifecycle de comando

Mensagens:

- `ack` significa recebido e pre-validado, nao aplicado com sucesso
- `reject` significa recusado
- `delta` significa estado aplicado
- `position_correction` reconcilia movimento
- `entity_appear` e `entity_disappear` alteram known-set
- `region_context` inicializa contexto

### Runtime state

Em memoria:

- active sessions
- socket/session/character binding
- region membership
- position atual
- facing
- active movement path
- waypoint atual
- accepted destination
- geodata version usada no path
- known-set
- target atual
- cast state
- cooldown map
- region-local entities
- region revision counters

No Postgres:

- accounts
- credentials
- characters
- level/XP
- HP/MP duravel
- position checkpoint
- region checkpoint
- geodata version/static terrain content quando persistido pelo pipeline de conteudo
- cooldown end timestamps duraveis
- inventory/equipment
- sessions duraveis
- command dedup records

Nao persistir microeventos:

- cada frame de movimento
- cada coordenada intermediaria
- cada waypoint advance
- cada path smoothing decision
- tick de cooldown
- tick de cast
- aparicao/desaparicao de known-set por frame
- mudanca visual transiente

## Gaps criticos atuais

Resolver estes antes de qualquer ambiente publico.

### Ja resolvido na consolidacao recente

- documentacao local de Docker/Postgres foi alinhada
- source root local absoluto foi removido das docs publicas de estudo
- backlog online foi atualizado para refletir o que ja esta implementado
- checklist de readiness online foi criado
- novas credenciais usam `bcrypt_v1`
- credenciais legadas `sha256` sao aceitas apenas como boundary de migracao e promovidas no login bem-sucedido
- access sessions sao duraveis e possuem expiry
- register, login e attach possuem rate limit minimo em memoria
- CORS HTTP direto falha fechado sem allowlist e aceita apenas origens configuradas
- valores sensiveis nao devem ser logados

Riscos residuais de auth/sessao:

- rate limit ainda e in-memory e single-instance
- ainda nao existe endpoint HTTP explicito de logout/revocation
- sessoes expiradas sao rejeitadas na leitura, mas ainda nao ha limpeza operacional dedicada
- fan-out cross-instance amplo ainda nao existe; a foundation atual classifica local, remote-online e offline, entrega notice de target, whisper/region chat, notices party/clan e projecao visual versionada de player/movimento via outbox PostgreSQL, mas party chat, combate remoto e replicacao autoritativa continuam pendentes

Foundation multi-instancia de sessao ja resolvida:

- `gameplay_session_ownerships` persiste `server_instance_id`, session, personagem, regiao, deadline de lease e fencing token monotono
- attach concorrente serializa por personagem no PostgreSQL; a credencial e rotacionada atomicamente e so uma tentativa com o mesmo token vence
- reconnect autenticado da mesma session na instancia owner drena dispatch serializado, avanca o fence e invalida o socket anterior; outra instancia/session nao substitui owner com lease valido
- comandos renovam e validam o fence antes de dedup/ack; owner antigo recebe `session.stale_owner`
- startup de uma instancia nao fecha ownership valida de outra instancia
- release/unregister e condicional e idempotente; double unregister nao derruba gauge nem runtime
- release preserva tombstone expirada para que o proximo acquire continue o fencing monotono do personagem
- ownership events possuem metrica e log estruturado sem attach token
- `gameplay_event_outbox` persiste eventos com id monotono, idempotency key imutavel, destino exato de instancia, payload JSON limitado, claim lease, retry, delivery e dead-letter
- `gameplay_event_receipts` persiste destinatario, instancia, delivery e consume por `event_id`; receipt consumido sobrevive a restart logico e dois consumers nao duplicam entrega
- finalizacao do command record e producao de todos os eventos remotos acontecem na mesma transacao; whisper/region chat incluem historico sanitizado na mesma fronteira, replay identico nao duplica history/evento/mensagem e replay conflitante continua rejeitado
- comandos party/clan incluem mutation autoritativa nessa mesma transacao e adiam fanout local ate o commit; expiry por disconnect permanece fallback idempotente separado e pode perder apenas o notice em crash entre delete/producao
- poller por instancia usa `FOR UPDATE SKIP LOCKED`, revalida ownership do destinatario e nao bloqueia o pipeline principal de gameplay
- retention remove somente eventos entregues antigos; falhos permanecem para retry ou dead-letter
- drift de ownership nao faz reroute implicito: destinatario offline/stale segue retry/dead-letter com reason interno estavel, sem fallback local; a validacao inclui o fencing token capturado, entao reuso da mesma session sob fence novo nao cruza takeover
- runtime e read-model mantem dedup limitado por `event_id`; party/clan reidratam delta autoritativo antes do notice remoto
- `presence.region_player_projection.v1` publica upsert/despawn exato por ownership remoto da mesma regiao; fence + versao monotona impedem overwrite/resurrection stale, heartbeat repara snapshot e TTL remove visual sem conceder autoridade
- a fila runtime de publicacao e limitada, independente do dispatcher e possui coalescing bounded do ultimo snapshot por source; pressao/drop e atraso sum/count/max sao observaveis, e o consumer revalida recipient fence e source ownership antes de projetar apenas identity/appearance/position/facing/movement/target visual necessarios
- o profile Compose `multi-backend` e seu Playwright real validam ownership separado, projecao/chat bidirecional, burst, stop/restart, retry/dead-letter, receipt, TTL/despawn, tombstone sem resurrection stale, reconnect e recovery
- reconnect da mesma gameplay session recebe `next_command_seq` derivado do maior command record duravel, preservando o namespace de replay em vez de reiniciar localmente em 1
- o socket nao compartilha transacao com PostgreSQL: crash depois do send e antes de `consumed_at` ainda pode redeliver apos restart simultaneo de server/page; exactly-once exigiria ack duravel do client

### Ja resolvido na Fase C - Deduplicacao de comandos

Implementado:

- dedup por `session_id + command_seq`
- `command_id` estavel em retry
- replay mesmo seq/id retorna outcome anterior sem reaplicar side effect
- mesmo seq com command_id diferente rejeita conflito
- persistencia de dedup records em `gameplay_command_records`
- status minimo `pending`, `rejected`, `applied`
- replay de `use_skill`, `pick_up_loot`, `equip_item` e `unequip_item` sem duplicar side effects
- adapters de memoria e PostgreSQL implementados

Riscos residuais:

- teste de restart PostgreSQL depende de `L2BG_TEST_DATABASE_URL` e deve rodar em ambiente com banco de teste real
- outcome de comando que falha durante finalizacao do dedup pode ficar pendente e precisa de observabilidade/operacao
- reconnect sofisticado alem de reissue autenticado, rotacao de credencial e replacement fenced da mesma session continua fora de escopo

### 1. Observabilidade

Docs exigem sinais minimos, mas codigo ainda nao tem camada robusta.

Implementar:

- structured logs por request e comando
- request latency
- command latency
- contador de rejects por reason_code
- contador de acks/deltas
- active websocket sessions
- login/register/character creation counters
- session attach counters
- region occupancy
- DB error counters
- job queue depth quando jobs existirem

Preferir OpenTelemetry-compatible, mas com interface interna.

### 2. Migrations

Estado atual:

- `ops/db-init/bootstrap.sql` idempotente

Evoluir para:

- migrations versionadas
- runner local simples ou ferramenta leve
- migration checks em CI/local
- rollback/forward-fix documentado

Nao bloquear tudo por migration framework pesado. Priorizar simplicidade.

### 3. E2E real

Adicionar validacao ponta a ponta:

- docker compose sobe
- frontend acessa backend via `/api`
- register
- login
- create character
- enter world
- attach WebSocket
- recebe region_context
- move
- select target
- use skill
- loot pickup por clique direto e por `pick_up_nearby`, incluindo aproximacao autoritativa quando estiver fora do range imediato
- equip/unequip

Pode ser Playwright se fizer sentido, mas adicionar dependencia apenas com script claro e docs.

### 4. Persistencia completa de gameplay

Estado atual:

- inventory/equipment persistem
- world position checkpointa em boundary
- loot pickup persiste item somente quando o runtime autoritativo entra em range e coleta o drop
- HP/MP persistem no fluxo online atual
- XP/level persistem via combate online
- cooldown end timestamps persistem quando aplicavel
- morte/respawn simples tem checkpoint duravel
- class stats e learned skills derivam de catalogo/backend
- hotbar snapshot autoritativo existe em `world/enter` e deltas
- comando autoritativo `set_hotbar_state` persiste rebind de `skill`, `item`, `action`, slot vazio e `open_bar_count`

Fechado recentemente:

- quest state duravel em snapshot, runtime e persistencia
- NPC services autoritativos para wardkeeper, merchant e warehouse
- comando autoritativo `use_item` para consumiveis na shortcut/action bar

### 5. Region presence e multiplayer real

Estado atual:

- known-set por runtime
- fan-out de loot appear/disappear entre sessoes na mesma regiao
- presence de players
- player appear/disappear
- movement delta de outros players
- region occupancy metrics
- ownership persistente por personagem com `server_instance_id`, lease renovavel e fencing token
- classificacao minima `local`, `remote-online`, `offline` e `unavailable` sem persistir known-set
- `select_target` e PvP falham com `presence.target_remote` quando o player conhecido esta em outra instancia; party/clan podem revalidar e convidar um target selecionado autoritativamente antes do drift de ownership
- `select_target` remoto produz um notice informativo replay-safe para a instancia owner sem mudar target local, habilitar dano ou substituir `presence.target_remote`
- whisper remoto, region chat e notices de invite/accept/decline/leave/kick/dissolve party/clan usam o outbox com chave por command/purpose/recipient e sem sucesso local falso
- receipts duraveis fecham redelivery apos restart do consumer, e mutation party/clan + outcome + outbox agora commitam ou rollbackam juntos

Ainda falta:

- region transfer
- interest management real
- interest management mais fino, party chat, combate remoto e replicacao de mobs/NPC/loot/pets
- ack duravel de consume no protocolo apenas se a ambiguidade residual socket/receipt justificar; reroute continua proibido sem contrato explicito
- alliance social base

### 6. Resend e emails transacionais

Docs ja definem:

- Resend e primeiro adapter, nao dependencia do core
- emails fora do caminho critico de gameplay
- emails partem de committed domain events ou notification intents

Ainda falta:

- tabelas `notification_intents`, `email_messages`, `email_provider_events`, `jobs`
- worker de jobs Postgres
- porta interna de email
- adapter Resend
- webhook endpoint com verificacao de assinatura
- templates iniciais
- retry/backoff limitado
- metricas de delivery

Fluxos iniciais:

- account verification
- password recovery
- login security notification se necessario
- transactional events nao criticos do jogo

### 7. Seguranca e integridade

Estado atual:

- password hashing com bcrypt e upgrade de SHA-256 legado em login bem-sucedido
- access sessions duraveis com expiry em modo PostgreSQL
- rate limiting minimo em auth/attach
- CORS configuravel por env
- command reason-code metrics
- protecao de economia contra client price nos fluxos implementados
- audit trail consultavel para vendor, exchange, sell, warehouse e player-trade com filtros read-only internos e token dedicado

Ainda falta:

- password policy real
- verification/recovery real
- token/session revocation
- HTTPS/WSS em producao
- input validation central por DTO
- anti-abuse logs
- checagem de origem WebSocket robusta
- alerting, correlacao e operacao anti-abuse acima das consultas minimas ja entregues

### 8. Conteudo e sistemas de jogo

Estado atual:

- quest persistence duravel
- NPC services autoritativos para merchant, warehouse e wardkeeper
- comando autoritativo `set_hotbar_state` para rebind persistente de `skill`, `item` e `action`
- comando autoritativo `use_item` para consumiveis em inventario e hotbar

Ainda falta construir:

- pets/mounts/taming
- party
- clans
- alliances
- PvP
- PK/karma
- instancias
- castle siege
- olympiad
- eventos PvP
- admin/liveops

### 9. Terrain, geodata e pathfinding server-side

Estado atual:

- geodata server-owned por regiao no slice online atual
- static blockers para pedras, paredes, ruinas, cercas, penhascos e obstaculos similares
- pathfinder no backend para gerar rotas alternativas
- reject/correction estavel para destino bloqueado ou inalcancavel
- reason codes `movement.*`
- delta/correction com rota autoritativa
- client mantendo prediction, pending path, authoritative path e blend suave como estado interno; linhas visuais de path/geodata sao debug-only e ficam desabilitadas no gameplay normal
- testes deterministas sem rede/db para pathfinding
- E2E/integracao validando contornar obstaculo e falhar quando nao ha rota

Regra central:

- `move_intent` continua enviando apenas `point`
- backend deriva navigability/path/collision
- client nunca envia path/waypoints/collision_result

## Roadmap executor recomendado

Trabalhar em fatias verticais. Cada fatia deve terminar com docs, testes e validacoes.

### Fase A - Consolidacao imediata

Status: concluida.

Objetivo:

- alinhar documentacao com codigo
- remover confusoes
- fechar gaps obvios de contrato

Tarefas:

1. corrigir docs defasadas
2. atualizar backlog Fase 1.1/Fase 1.2 para refletir implementado/parcial/falta
3. criar checklist real de pronto online
4. revisar skills para garantir que apontam para este master prompt quando util
5. rodar validacoes

Done:

- docs nao mentem sobre Postgres
- nenhum source root local absoluto em docs publicas
- backlog reflete estado atual
- testes passam

### Fase B - Auth e sessao minimamente seguros

Status: concluida para o minimo de dev/staging inicial.

Objetivo:

- trocar atalhos inseguros por base aceitavel para staging

Tarefas:

1. substituir SHA-256 simples por Argon2id ou bcrypt
2. versionar password algorithm
3. criar account session/access token duravel com expiry
4. implementar logout/revocation se escopo for pequeno
5. adicionar rate limit basico por IP/login em memoria primeiro ou Postgres conforme simplicidade
6. CORS configuravel por env
7. testes de auth

Done:

- senha nao usa SHA-256 simples
- token expira
- restart nao quebra fluxo esperado indevidamente, ou comportamento documentado
- tests passam

Riscos residuais:

- rate limit ainda e in-memory/single-instance
- logout/revocation ainda nao possui endpoint HTTP publico
- limpeza operacional de account sessions expiradas ainda nao existe
- metricas estruturadas de auth/attach ainda pertencem a fase de observabilidade

### Fase C - Command dedup duravel

Status: concluida.

Objetivo:

- cumprir spec oficial de replay/idempotencia

Tarefas:

1. migration/tabela de command records
2. integrar pre-validation com dedup
3. registrar accepted/rejected/applied outcome
4. rejeitar conflicting replay
5. reapresentar outcome em retry identico
6. testes unitarios e integracao

Done:

- spec `command-envelope` atendida
- side effects nao duplicam em retry
- testes cobrem conflito e replay

Riscos residuais:

- teste de restart PostgreSQL precisa de `L2BG_TEST_DATABASE_URL` configurado para rodar de verdade
- observabilidade de outcomes/reason codes ainda pertence a Fase E

### Fase D - E2E Docker online

Status: concluida para o fluxo online atual.

Objetivo:

- provar que o jogo online sobe e joga do navegador/local

Tarefas:

1. adicionar script de E2E
2. subir Compose em teste quando ambiente permitir
3. testar register/login/create/enter/attach
4. testar movement/target/skill/loot/equip
5. documentar como rodar

Done:

- E2E reproduz fluxo online completo
- falhas geram diagnostico claro

### Fase E - Observabilidade minima

Objetivo:

- tornar o runtime mensuravel

Tarefas:

1. criar porta interna de metrics/logging
2. instrumentar HTTP
3. instrumentar WebSocket attach
4. instrumentar command lifecycle
5. instrumentar DB errors
6. expor endpoint `/metrics` ou integracao OTel simples
7. docs de sinais

Done:

- operators conseguem ver fluxo quebrando
- reason_code aparece em contadores

### Fase F - Persistencia de progressao online

Objetivo:

- online deixar de ser apenas combate runtime parcial

Tarefas:

1. character HP/MP duravel
2. XP/level duravel
3. cooldown ends duravel
4. death/respawn checkpoint
5. quest state duravel
6. tests de restart/world enter

Done:

- sair/entrar preserva estado relevante
- nao persiste microeventos

### Fase G - Multiplayer presence real

Objetivo:

- outros players aparecem e se movem

Tarefas:

1. runtime region registry
2. player entity appear/disappear
3. movement broadcast por known-set
4. target legality considerando players quando necessario
5. region occupancy metric
6. tests multi-session

Done:

- duas sessoes veem entidades relevantes
- movimento de outro player aparece
- logout/close limpa presence
- duas instancias concorrendo pelo mesmo personagem produzem um owner duravel
- reconnect da mesma session na instancia owner drena dispatch, rotaciona a credencial, incrementa o fence e invalida o owner anterior; takeover cross-instance exige release ou expiracao
- comando do owner antigo rejeita antes de ack/dedup
- presence minima distingue player remoto online de offline/desconhecido sem fallback local

### Fase H - Conteudo de classe/skill/hotbar

Objetivo:

- sair de duas skills hardcoded para sistema expansivel

Tarefas:

1. class definitions
2. class template stats
3. skill definitions
4. active/passive categorization
5. skill book
6. hotbar persistente
7. learning/unlocking rules
8. tests de regra
9. use_item autoritativo para consumiveis na shortcut/action bar

Done:

- player tem skills por classe
- hotbar persiste
- comando autoritativo `set_hotbar_state` persiste mudancas iniciadas pela UI para `skill`, `item`, `action`, slot vazio e `open_bar_count`
- comando autoritativo `use_item` cobre consumiveis no inventario e na shortcut/action bar
- backend valida skill conhecida e aprendida
- HUD exibe familia de janelas do personagem `ALT+T/K/C/N/U`, com navegacao superior compartilhada
- HUD exibe skill book `ALT+K` com `Active`/`Passive`, slots `32x32px`, sem fileira duplicada de skills acima das categorias
- drag para hotbar mostra o icone da skill ou item preso ao cursor
- hotbar e uma shortcut/action bar multitarefa, nao uma barra exclusiva de skills

### Fase I - Inventario e economia ampliados

Objetivo:

- tornar item loop MMORPG

Tarefas:

1. slots adicionais
2. item attributes
3. stack split/merge
4. vendors
5. buy/sell backend-derived price
6. warehouse
7. trade/exchange depois
8. audit de economia

Done:

- client nunca envia preco
- equipamento muda stats e visual
- transacoes atomicas
- vendor buy/sell, exchange, warehouse e trade estao autoritativos
- `action_logs` e `storage_transfer_records` carregam actor, account, item, quantidade, currency delta, before/after e metadata de comando quando disponivel
- consultas internas read-only existem para eventos por personagem, item, action type, janela de tempo, warehouse transfers e trades

### Fase I.5 - Server terrain, geodata e pathfinding

Status: concluida para o slice online atual.

Objetivo:

- amadurecer colisao e movimento para nao depender de decisao visual do client
- permitir rotas alternativas automaticas ao redor de obstaculos
- manter click-to-point simples, imediato e fluido para o jogador
- manter o backend como autoridade final sem fazer o personagem esperar parado

Tarefas:

1. ler `docs/specs/server-terrain-geodata-pathfinding.md`
2. modelar geodata por regiao com versao, bounds, superficies navegaveis, blockers e exits/portals
3. criar porta de dominio para geodata/pathfinder sem acoplar a lib/framework
4. implementar pathfinder deterministico simples, testavel sem rede/db
5. validar `move_intent` contra geodata server-owned
6. gerar rota alternativa ao redor de pedras, paredes, ruinas, cercas, penhascos e obstaculos similares
7. rejeitar ou snappar destino bloqueado/fora de bounds/inalcancavel com reason code estavel
8. retornar rota/destino autoritativos via `delta`/`PositionCorrection`, sem protocolo paralelo
9. atualizar client para locomocao preditiva imediata, pending path, authoritative path e feedback blocked/unreachable
10. aplicar prediction leash para evitar drift visual grande antes da rota autoritativa
11. fazer blend suave da predicao para a rota do servidor, usando snap duro apenas em reject ou divergencia grande
12. garantir que pathfinding no backend tenha budget/cancelamento e nao bloqueie o loop de socket/regiao
13. usar goroutines/worker pool apenas se preservar `command_seq`, dedup, cancelamento de path antigo e aplicacao serializada de estado
14. garantir que retry/dedup de movimento nao gere outcome divergente
15. adicionar testes unitarios, integracao e, se viavel, E2E browser para obstaculo e responsividade do click-to-move

Done:

- client envia somente destino
- client-supplied path/waypoints/collision_result e rejeitado
- personagem local comeca a andar imediatamente apos click/dispatch em fluxo online normal
- resposta do backend reconcilia a predicao com blend suave quando possivel
- pathfinder contorna obstaculos quando existe rota
- destino inalcancavel gera reject/correction estavel
- runtime nao persiste movimento por frame/waypoint
- docs, specs, skills e backlog ficam alinhados

### Fase J - Quests e NPC services

Objetivo:

- construir profundidade Lineage-like com simplicidade

Tarefas:

1. quest definitions
2. quest state persistente
3. NPC interaction commands
4. rewards autoritativas
5. dialog state client-only, quest truth backend
6. quest tracker

Done:

- quest sobrevive reconnect
- rewards nao duplicam
- dialogo no client e apenas projecao de `quest` e `npc_interaction`
- merchant e warehouse usam interaction snapshot autoritativo
- HUD mostra tracker real apenas quando existe quest autoritativa

### Fase K - Pets, taming e mounts

Objetivo:

- implementar fantasia de domar monstros

Tarefas:

1. tameable monster profiles
2. tame command
3. tame conditions
4. companion instance persistence
5. summon/unsummon
6. mount/dismount
7. companion HUD
8. visual ownership markers
9. AI simples

Done:

- `mireling` elegivel pode virar `mireling_strider` como `pet_mount` canonico do slice
- ownership e persistente em `character_pets`
- `tame_mob`, `summon_pet`, `dismiss_pet`, `mount_pet` e `dismount_pet` sao comandos autoritativos
- companion summoned aparece no `known-set` e em `region_context` como entidade `pet`
- mounted state altera `move_speed` por derivacao backend e reidrata em `world/enter`
- `ALT+C` expoe atalhos autoritativos para tame, summon, dismiss, mount e dismount
- pet combat avancado, pet inventory, pet equipment, breeding e AI ampla ficaram fora deste slice

### Fase L - Social core

Objetivo:

- base MMO social

Tarefas:

1. chat social minimo por `region`
2. whispers se desejado
3. party
4. party presence
5. shared XP/loot rules
6. clan base
7. alliance depois

Done:

- primeiro slice autoritativo de `party` concluido
- o slice base de `party` agora segue o modelo canonico minimo: invite usa o target player atual, TTL de 10s, cap de 9 membros, invitee aceita ou recusa um convite efemero e a party so nasce ou cresce no aceite
- `parties`, `party_members` e `party_invites` persistem leader, membership e convite pendente; o invite continua efemero, no maximo um outbound ativo por inviter ou party e no maximo um invite pendente por invitee
- `invite_party_member`, `accept_party_invite`, `decline_party_invite`, `leave_party` e `kick_party_member` sao comandos autoritativos com dedup replay-safe
- `world/enter.self_state.party` e `world/enter.self_state.party_invites` reidratam roster e convites pendentes de forma coerente com a nova semantica de aceite
- membros recebem `party_notice` e delta de roster sem confiar em sucesso local; `party_notice` continua lifecycle feedback, nao verdade de estado
- HUD agora projeta painel compacto de party fechado por padrao via `ALT+P`, com drag, close, roster e leave; o invite recebido usa uma janela dedicada pequena, nao-dragavel, centralizada acima da hotbar, com `Accept`, `Cancel` e countdown visual derivado de `expires_at_ms`
- `ALT+C` expoe `party_invite` e `party_leave`, enquanto `/invite` e `/leave` existem apenas como affordances parseadas no cliente para gameplay commands autoritativos, sem misturar party logic com `send_chat_message`
- disconnect de inviter ou invitee cancela o convite pendente; self-invite, target ja em party, duplicate invite e party cheia rejeitam com `party.*`
- party nao existe funcionalmente com 1 membro: leave ou kick que derrubam para 1 dissolvem; se o leader sair com 2+ remanescentes, a lideranca passa deterministicamente ao membro restante mais antigo
- `send_chat_message` agora cobre `region`, `party` e `whisper` com validacao, trim, limite de tamanho, rate limit simples, dedup replay-safe e fan-out autoritativo
- `chat_message` entrega apenas para sessoes elegiveis por regiao, party online ou whisper target online/localizavel por nome; whisper e region chat remotos usam outbox PostgreSQL com owner/session exatos, sem fallback local de sucesso
- `chat_messages` persiste historico minimo server-side para auditabilidade futura com actor, account, canal, alvo quando houver, regiao quando aplicavel, texto saneado e metadata de comando; no whisper/region chat remoto, history + command outcome + eventos commitam atomicamente e fanout local espera o commit
- HUD de chat reutiliza a janela canonica no canto inferior esquerdo com filtros `All`/`Region`/`Party`/`Whisper`, Enter para focar/enviar e Esc para cancelar, sempre renderizando texto escapado
- estado morto nao bloqueia `send_chat_message` neste primeiro slice social; o backend continua autoridade de validacao, fan-out e persistencia
- `local` nao tem semantica distinta nesta fase e fica explicitamente reservado para depois; a superficie funcional atual exposta e testada e `region`, `party` e `whisper`
- `shared XP` e `loot sharing` agora existem no menor slice autoritativo sobre `party`: elegibilidade minima por party online/attached, mesma regiao e vivo; o backend divide XP de forma deterministica e marca loot de party com ownership minimo para pickup autoritativo
- pickup de loot de party continua first valid pickup wins entre elegiveis, persiste em `character_items` e rejeita ator fora da elegibilidade com reason code estavel
- a foundation minima de `clan` agora existe em modo autoritativo: `create_clan` cria o clan imediatamente com founder como leader e primeiro member; `invite_clan_member`, `accept_clan_invite`, `decline_clan_invite`, `leave_clan`, `kick_clan_member` e `dissolve_clan` seguem o command lifecycle replay-safe sem fallback local
- `clans`, `clan_members` e `clan_invites` persistem nome unico, leader, membership e convite efemero com TTL de 10s; disconnect de inviter ou invitee cancela o convite e `world/enter.self_state.clan` mais `self_state.clan_invites` reidratam a verdade compacta do slice
- targeting de player usado pelo social core agora passa por `select_target` autoritativo e delta correlacionado; isso nao habilita PvP/PK, e `invite_clan_member` aceita somente payload vazio sem recipient escolhido pelo client
- aceite de clan adiciona membership e consome o invite atomicamente; storage limita um outbound vivo por clan e um inbound vivo por invitee
- os sete comandos de clan tem cobertura de replay identico deterministico e replay conflitante sem mutacao, alem de invalid target, expiracao, disconnect, reconnect/hydration, kick e dissolve
- smoke real com dois personagens via Docker Compose cobre create, invite, accept, reconnect, leave, decline, novo accept, kick e dissolve sem sucesso local falso
- `ALT+N` agora projeta o clan base com `No Clan` mais `Create Clan`, roster compacto quando joined, acoes leader-only de `Invite`/`Kick`/`Dissolve`, `Leave` para member comum e modal dedicado de clan invite nao-dragavel acima da hotbar com countdown por `expires_at_ms`
- leader de clan nao pode usar `leave_clan` nesta fase; o clan continua valido com 1 member, `dissolve_clan` e explicito e leader-only, e manual transfer ou auto-transfer de leader continuam fora de escopo
- round-robin, master loot, dice UI, redistribution, penalty sofisticada por range/level, clan/alliance reward sharing, alliance, siege, clan war amplo, party finder, matchmaking, offline mail, moderacao avancada, clan chat real, clan warehouse, clan skills, academy, subunits, crest rico, privilegios complexos e transfer manual de leader seguem fora de escopo

### Fase M - PvP/PK

Objetivo:

- combate entre players com regras de risco

Tarefas:

1. [concluido no hardening] policy fail-closed por regiao mais santuario logico minimo de spawn em `stonecross_plaza`/`dawn_plaza`, avaliado apenas pelo backend sem alterar mapa/geodata/picking/bounds/spawn/assets; volumes ricos de content continuam futuros
2. [concluido no slice minimo] target player legality separada de `select_target`, com known-set, attach, regiao, vida, party, clan, range, skill, MP e cooldown backend-owned
3. [concluido no hardening] PvP flag de 30s persistida como deadline absoluto, refresh por hit hostil, reconnect/restart logico enquanto valida e expiracao server-owned com limpeza duravel
4. [concluido no slice minimo] classificacao de kill PvP versus PK no instante da morte
5. [parcial] `pvp_kills`, `pk_count` e karma fixo persistentes; decay, recovery e penalidades economicas seguem futuras
6. [concluido no hardening minimo] morte e respawn simples backend-owned com limpeza de target ofensivo, ataques queued/auto, loot approach, movement, flag e cooldown; perdas de XP/item e outras penalties seguem futuras
7. [parcial] assist/attribution por hits aplicados em 30s e sinal `suspicious` para kill repetida do mesmo par em 10min estao concluidos; protecao por level, correlacao account/device, bloqueio e automacao anti-grief seguem futuras
8. [concluido no hardening concorrente] cada hit/kill trava attacker e victim em ordem deterministica no PostgreSQL e persiste dano/MP/cooldown/death/flag/counters/karma/attribution/anti-feed/audit como unidade; query interna read-only reutiliza token de audit e filtra killer/victim/suspicious/action/result/time

Done:

- PvP nao quebra PvE
- `select_target` de player nao causa dano nem habilita sucesso local
- `basic_attack` e skill single-target contra player passam por lifecycle/dedup duravel e nao reaplicam side effects no replay
- dano consome CP antes de HP; morte, classificacao, cooldown clear e respawn sao backend-owned
- PK tem consequencia minima duravel por `pk_count + karma`; kill de alvo exposto ou karma-positive conta como PvP
- lock process-local e apenas coordenacao de runtime; a garantia multi-instancia vem de transacao PostgreSQL com row lock dos dois combatentes e calculo sobre verdade duravel recarregada
- hits recentes compoem ledger duravel de attribution: ultimo golpe letal define killer, attackers distintos em 30s viram assists e a morte anterior da vitima corta a janela
- segunda kill do mesmo killer/victim em 10min marca `suspicious` e `repeated_kill_count` no audit, sem bloquear gameplay nem alterar rewards
- policy de regiao desconhecida falha fechada e santuario logico de spawn bloqueia actor ou target com `pvp.safe_zone`; isso nao altera renderer, mapa, picking, geodata, bounds, spawn ou checkpoint
- siege, olympiad, clan/alliance war, eventos, ranking, reward e penalidade economica complexa continuam fora

### Fase N - Instancias, siege e olympiad

Objetivo:

- endgame competitivo

Ordem:

1. instancia simples
2. PvP event simples
3. clan ownership primitives
4. castle data model
5. siege scheduling
6. siege runtime
7. olympiad matchmaking
8. hero/reward loop

Done:

- cada sistema tem runtime, persistencia, calendario e reward auditavel

### Fase O - Producao

Objetivo:

- preparar para usuarios reais

Tarefas:

1. HTTPS/WSS
2. secrets fora do repo
3. PgBouncer
4. backups/restore drill
5. OpenTelemetry/dashboards
6. alertas
7. load test 2k active players
8. runbooks
9. staging
10. deploy Linux

Done:

- release gates passam
- ambiente staging reproduz producao
- load test mede gargalos reais

## Regras para escolher a proxima tarefa

Sempre escolher a maior prioridade que:

1. desbloqueia mais fatias futuras
2. reduz risco de arquitetura
3. tem criterio de teste claro
4. evita rework
5. mantem o jogo jogavel

Prioridade atual recomendada:

1. limitar e superseder com seguranca snapshots de projecao duraveis obsoletos expostos pelo backlog medido em fault, depois aprofundar interest management, sem combate remoto, Redis ou fila externa antes de necessidade comprovada
2. karma recovery, correlacao account/device e alerting sobre attribution/anti-feed ja auditados, sem ampliar para guerras/eventos ou aplicar punicao automatica ainda
3. instancias, siege, olympiad e producao somente depois de ownership, presence cross-instance, PvP/PK e clan permanecerem estaveis

Nao pular para siege/olympiad antes de:

- auth segura
- sessoes solidas
- command lifecycle solido
- terrain/geodata/pathfinding server-side solido
- inventory/economia auditavel
- PvP basico
- clan basico

## Workflow obrigatorio de cada fatia

### 1. Orientar

Rodar:

```bash
git status --short
rg --files
```

Ler:

- este arquivo
- docs relevantes da fatia
- codigo relevante
- testes existentes

### 2. Planejar curto

Definir:

- objetivo da fatia
- arquivos que serao alterados
- contratos afetados
- testes necessarios
- docs/skills afetadas

Nao criar plano enorme se a fatia for simples.

### 3. Implementar

Regras:

- mudanca pequena e completa
- sem overengineering
- manter ports/adapters
- dominio testavel sem rede/db quando possivel
- persistencia em transacao curta
- reason codes estaveis
- client apenas envia intencao
- docs atualizadas no mesmo PR/fatia

### 4. Testar

Minimo:

```bash
docker compose exec -T frontend npm run typecheck
docker compose exec -T frontend npm test -- --run
docker compose exec -T frontend npm run build
docker compose exec -T backend go test ./...
docker compose exec -T backend go build ./cmd/server
docker compose config --quiet
```

Quando tocar Docker/Postgres:

```bash
docker compose up -d --build
```

Se nao puder rodar Compose por ambiente:

- informar claramente
- rodar o maximo possivel localmente
- nao fingir validacao

### 5. Corrigir

Se teste falhar:

- ler erro
- corrigir causa
- repetir teste relevante
- depois rodar suite ampla

### 6. Documentar

Atualizar docs quando mudar:

- arquitetura
- contrato HTTP/WS
- payload
- reason codes
- schema
- runtime state
- security boundary
- client flow
- deploy/op
- sistema de jogo
- skill/subagent guidance

### 7. Fechar

Registrar:

- o que foi feito
- testes rodados
- riscos residuais
- proxima fatia recomendada

Mas se estiver operando como executor continuo, seguir para proxima fatia sem pedir permissao, exceto nos criterios de parada.

## Criterios de parada

Pedir input ao usuario apenas se:

- precisar de credencial Resend, dominio, secret, conta externa ou pagamento
- houver decisao artistica nao mapeada e nao inferivel
- houver conflito entre docs atuais
- houver risco legal/IP/uso de marca/asset
- precisar acessar source externa que nao esta no repo
- uma decisao muda radicalmente produto ou arquitetura
- o ambiente local impossibilita validacao essencial

Nao pedir input para:

- nome de tabela obvio
- organizacao interna de pacote
- qual teste escrever
- como corrigir bug claro
- atualizar docs defasadas
- escolher implementacao simples entre opcoes equivalentes
- adicionar reason code coerente
- criar migration necessaria
- melhorar cobertura de teste

## Criterios de pronto por tipo de mudanca

### Backend command

Pronto quando:

- comando usa envelope oficial
- actor derivado de sessao
- input validado
- client nao envia resultado
- se for movimento, client nao envia path/waypoints/collision_result
- dominio testado
- side effects transacionais
- rejects com reason_code
- observabilidade adicionada se caminho critico
- docs/specs atualizadas

### HTTP endpoint

Pronto quando:

- metodo correto
- auth correta
- DTO interno limpo
- erro com reason_code
- nao vaza provider/framework
- tests de sucesso e falha
- docs atualizadas

### WebSocket flow

Pronto quando:

- attach validado
- origem validada
- primeira mensagem obrigatoria quando aplicavel
- close limpa sessao/presence
- mensagens seguem contrato
- tests cobrem reject e sucesso

### Schema/persistencia

Pronto quando:

- migration ou bootstrap atualizado
- constraints corretas
- indices para query path
- transacao curta
- testes com Postgres/memoria se aplicavel
- docs atualizadas

### Frontend feature

Pronto quando:

- client nao vira autoridade
- estado online vem do read model
- estado local continua explicitamente local
- UI mostra erro/reject
- tests de reducer/read model/HUD
- build passa
- UX nao cria fallback silencioso
- movimento visual reconcilia com rota/correcao do backend quando aplicavel

### Sistema de jogo

Pronto quando:

- regra documentada
- comando/estado definidos
- persistencia definida se duravel
- runtime definido se efemero
- client feedback definido
- se tocar movimento, terrain/geodata/pathfinding definido e testado
- tests de balanceamento minimo
- edge cases cobertos

## Padroes de reason_code

Usar namespaces:

- `auth.*`
- `character.*`
- `session.*`
- `protocol.*`
- `sequence.*`
- `world.*`
- `presence.*`
- `combat.*`
- `inventory.*`
- `loot.*`
- `economy.*`
- `movement.*`
- `quest.*`
- `pet.*`
- `mount.*`
- `chat.*`
- `party.*`
- `clan.*`
- `pvp.*`
- `system.*`

Exemplos existentes:

- `auth.not_authenticated`
- `auth.invalid_credentials`
- `auth.account_unverified`
- `auth.account_locked`
- `character.invalid_race`
- `character.name_unavailable`
- `session.character_already_active`
- `session.invalid_attach_token`
- `session.ownership_conflict`
- `session.stale_owner`
- `presence.target_remote`
- `presence.target_offline`
- `protocol.invalid_envelope`
- `sequence.out_of_order`
- `world.entity_not_known`
- `combat.target_required`
- `combat.target_out_of_range`
- `combat.cooldown_active`
- `inventory.item_not_found`
- `loot.already_collected`
- `movement.destination_blocked`
- `movement.destination_out_of_bounds`
- `movement.path_unreachable`
- `movement.path_budget_exceeded`
- `movement.geodata_unavailable`
- `movement.geodata_mismatch`

## Contratos de seguranca

Antes de staging publico:

- senha com Argon2id/bcrypt
- access sessions com expiry real
- rate limit auth/attach
- CORS configurado por env
- HTTPS/WSS obrigatorio em producao
- secrets fora do repo
- logs sem segredos
- validation de input
- WebSocket origin control
- audit de economia
- provider webhooks com assinatura

Falhar fechado:

- auth duvidoso rejeita
- token invalido rejeita
- target desconhecido rejeita
- path/colisao vindo do client rejeita
- preco client-side rejeita
- command_seq conflitante rejeita
- provider webhook nao verificado rejeita

## Contratos de dados

Manter separado:

- template imutavel
- instance mutavel
- runtime efemero
- checkpoint duravel
- presentation state

Exemplos:

- `item_templates` nao mudam por personagem
- `item_instances` pertencem a personagem/container
- `equipment_occupancy` deriva de container/equip_slot
- `known-set` nao e duravel
- `target_id` online e runtime
- `geodata_version` e server-owned
- `active movement path` e runtime
- `path waypoints` nao sao persistidos por frame
- `cooldown end timestamp` e duravel quando necessario
- `cast countdown` e runtime

## Contratos de interface

Cena 3D:

- mundo
- personagens
- mobs
- NPCs
- loot
- target rings
- movement markers
- path/geodata debug markers somente quando habilitados explicitamente para desenvolvimento
- blocked movement feedback
- AoE previews
- floating feedback

HUD:

- player frame
- target frame
- hotbars compactas `32x32px`
- inventory/equipment em janela classica
- quest tracker apenas quando houver sistema de quest real, sem task tracker default
- system log/chat
- minimap/region
- skill book
- pet/mount panel
- modals

Nao misturar:

- HUD complexa dentro do Three.js sem necessidade
- regras de gameplay dentro de componente visual
- resultado autoritativo no client
- pathfinding/collision autoritativo no client

Regras visuais atuais da HUD:

- janelas classicas usam cantos quadrados, corpo escuro, barra azul e controles pequenos
- itens e skills aparecem como icones `32x32px`, com detalhes em hover/focus
- inventario abre/fecha com `ALT+V` e fica fechado por padrao
- painel de Skills abre/fecha com `ALT+K` dentro da familia de janelas do personagem
- janelas do personagem usam `ALT+T` Status, `ALT+K` Skills, `ALT+C` Actions, `ALT+N` Clan e `ALT+U` Quests
- o topo da janela do personagem e navegacao entre paineis, nao uma fileira de skills duplicadas
- skills ativas e itens do inventario podem ser arrastados para a shortcut/action bar como affordance local, com o icone seguindo o cursor durante o drag
- atalhos existentes na shortcut/action bar podem ser removidos arrastando para fora da barra ou usando `ALT + clique esquerdo` no slot ocupado
- equipaveis na barra executam equip; consumiveis executam `use_item`; actions do `ALT+C` executam comando autoritativo, com `basic_attack` movendo ate o target quando necessario, `pick_up_nearby` coletando o drop proximo sem target obrigatorio e `party_invite` ou `party_leave` reutilizando o mesmo fluxo autoritativo de party
- rebind de hotbar online so vira verdade duravel depois de comando backend autoritativo
- nao reintroduzir cards arredondados, grids `64x64px`, task tracker default ou controles decorativos sem clique

## Contratos de arte

Manter:

- lowpoly
- dark fantasy
- misterio
- silhueta clara
- legibilidade
- equipamentos alterando visual
- personagens grandes o suficiente para leitura

Evitar:

- textura realista detalhada
- excesso de particula
- UI generica
- assets que parecam copia de IP existente

## Load target

Meta inicial:

- pelo menos 2 mil pessoas jogando

Interpretacao tecnica:

- nao significa 2 mil players numa unica regiao pequena
- escalar por regioes e known-set
- medir comandos por segundo
- medir WebSocket connections
- usar Postgres com pool adequado/PgBouncer
- Redis so se fan-out/presence multi-instancia exigir

Load test futuro deve simular:

- login bursts
- character entry
- movement intents
- pathfinding ao redor de obstaculos
- blocked/unreachable movement attempts
- target selection
- skill use
- loot pickup
- inventory mutations
- region changes
- chat/social quando existir

## O que nao fazer

Nao:

- reescrever tudo por gosto
- introduzir framework grande sem necessidade
- trocar Postgres por Scylla
- adicionar Redis no core path sem metrica
- criar microservices cedo
- copiar source estudada literalmente
- deixar docs mentirem
- aceitar preco vindo do client
- aceitar character_id em comando gameplay
- aceitar path/waypoints/collision_result vindo do client
- implementar colisao apenas no Three.js como verdade de gameplay
- criar retry loop local para pickup, ataque ou interacao para esconder falha de autoridade
- persistir movimento frame a frame
- persistir cada waypoint advance
- usar fallback local quando online falha
- ignorar teste quebrado
- declarar validacao feita sem rodar

## Estado final desejado

O jogo e considerado finalizado para uma primeira versao publica quando:

- usuario registra/verifica/loga
- cria personagem
- entra no mundo online
- ve outros players relevantes
- move por click-to-point
- contorna obstaculos por pathfinding server-side
- recebe feedback claro para destino bloqueado ou inalcancavel
- seleciona target por click
- usa skills single-target e AoE com autoridade backend
- progride por XP/level
- aprende/usa skills por classe
- equipa itens que mudam stats e visual
- faz quests persistentes
- coleta loot autoritativo
- usa NPC services basicos
- tem inventario/equipment/warehouse basico
- pode participar de party
- tem PvP/PK basico com regras
- tem clan base
- tem pelo menos um loop de instancia/evento ou boss
- tem pets/mounts ou taming em versao jogavel
- tem observabilidade minima
- tem deploy Linux documentado
- passa testes automatizados
- passa E2E online
- passa load test definido para meta inicial
- tem runbooks e backup/restore testado

Sistemas avancados como castle siege e olympiad podem ser considerados expansao pos-primeira versao publica se o usuario aceitar, mas a arquitetura deve estar preparada para eles.

## Primeiro comando recomendado para o executor agora

Use este prompt no outro chat executor:

```text
Voce esta trabalhando dentro deste repositorio. Leia primeiro TRAE_SOLO_MASTER_PROMPT.md e trate-o como contrato operacional. Leia tambem docs/backlog/online-slice-now-next-later.md, docs/specs/pvp-pk.md, docs/specs/server-terrain-geodata-pathfinding.md, docs/specs/hud-skills-and-hotbars.md, docs/specs/hud-inventory-and-classic-windows.md, docs/interface-architecture.md e skills/threejs-client-engineer/references/client-implementation-guidelines.md.

Antes de alterar qualquer arquivo, rode git status --short e inspecione o estado real do codigo com rg. Nao assuma que uma fase esta pendente so porque o texto antigo dizia isso; compare docs, testes e implementacao real.

Estado atual que voce deve tratar como entregue, salvo evidencia contraria no codigo:
- A foundation de ownership multi-instancia ja persiste um lease por personagem, `server_instance_id` e fencing monotono; attach concorrente tem um vencedor, stale owner rejeita antes de ack/dedup e release/unregister e condicional/idempotente.
- Presence minima ja distingue local, remote-online e offline. O fan-out entre processos entrega notice de target, whisper remoto, region chat, notices party/clan e projecoes visuais versionadas de player/movimento na mesma regiao via outbox PostgreSQL; `presence.target_remote` continua bloqueando select/PvP e nao existe fallback local.
- O outbox cross-instance ja possui id monotono, chave idempotente imutavel por command/purpose/recipient ou source-fence-version/recipient-fence, producao atomica com command outcome quando aplicavel, whisper/region-history atomico, fanout regional local pos-commit, mutation party/clan na mesma transacao, claim seguro por instancia, receipts duraveis de delivery/consume, ordering de projecao, dedup runtime/read-model, retry/dead-letter, retention e observabilidade sem payload sensivel.
- O profile Compose `multi-backend` ja valida dois processos reais com ownership separado, projecao/chat bidirecional, burst, stop/restart, retry/dead-letter, receipts, TTL/despawn, tombstone, fence de recipient e recovery; a fila de publicacao e bounded/coalescing e mede pressao e atraso.
- Reconnect reutilizando gameplay session recebe `next_command_seq` duravel do backend; eventos remotos capturam recipient fence e nao atravessam takeover apenas porque o session id permaneceu igual.
- Fase L ja possui party canonica minima, social chat `region`/`party`/`whisper`, shared XP minimo, party-owned loot minimo e clan foundation hardened em slices autoritativos.
- Fase M ja possui o primeiro slice PvP/PK autoritativo hardened para ataque e skill single-target, CP antes de HP, deadline de flag persistido, safe-area minima backend-only, classificacao PvP versus PK, counters/karma duraveis, row locking PostgreSQL multi-instancia, attribution de killer/assists, sinal de kill repetida, audit investigavel, morte/respawn backend-owned e replay duravel.
- A regiao ativa atual usa `stonecross_plaza` apenas como id compativel, mas o mapa oficial foi resetado para uma area limpa 1024x1024 com `clean_plain_1024_geo_v1`, bounds `x=-512..512` e `z=-512..512`.
- Renderer, ground raycast/picking plane, server geodata bounds, spawn/checkpoint, exits e testes precisam compartilhar esse mesmo contrato de mapa.
- Nao reintroduza clamp hardcoded do mapa antigo, visual Stonecross, props/spawns antigos, blockers antigos, nem bounds antigos de `dawn_plaza`.

Prioridade 1: limitar e superseder com seguranca snapshots duraveis de projecao obsoletos expostos pelo backlog medido durante falha, depois aprofundar interest management/party chat, preservando `presence.target_remote` para combate e sem introduzir Redis/fila externa antes de medir necessidade.
- Reuse runtime autoritativo, persistencia curta e HUD classica.
- Preserve toda autoridade de sessao, presence, party, chat, clan, shared XP, party loot, elegibilidade PvP, dano, morte e consequencias no backend.
- Nao aceite membership, reward split, target legality, presence truth, chat delivery ou loot ownership vindo do client.
- O santuario PvP minimo atual e policy backend-only. Se uma proxima fatia exigir volumes de content ou tocar mapa, geodata, movement ou picking, atualize renderer/picking/backend/tests juntos; caso contrario, nao mexa no mapa.
- Nao crie microservicos, brokers ou filas externas sem necessidade medida; o outbox PostgreSQL atual e a foundation permitida.

Prioridade 2: depois disso, aprofunde karma recovery, correlacao/alerting e penalidades simples somente com contrato explicito e testes de reconnect/replay.
- Preserve known-set, command_seq, dedup, observabilidade e persistencia auditavel.

Para cada fatia:
1. implemente uma mudanca vertical pequena e completa;
2. atualize docs/specs/skills se mudar contrato, arquitetura, UI, dados ou fluxo;
3. rode obrigatoriamente via Docker Compose quando aplicavel: docker compose up -d --build, docker compose exec -T frontend npm run typecheck, docker compose exec -T frontend npm test -- --run, docker compose exec -T frontend npm run build, docker compose exec -T backend go test ./..., docker compose exec -T backend go build ./cmd/server, docker compose config --quiet;
4. corrija falhas antes de seguir;
5. pare para pedir input apenas se houver decisao criativa nao documentada, credencial externa, risco legal/IP, conflito entre docs ou dependencia externa indisponivel.

Nao reintroduza UI generica: HUD classica, janelas quadradas, barras azuis, slots 32x32, icon-only grids e tooltips sao canonicos.
```
