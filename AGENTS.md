# AGENTS.md — Vassal vLog Sync

## Visão Geral

Sincroniza arquivos `.vlog` do [Vassal Engine](https://www.vassalengine.org/) entre jogadores automaticamente via um modelo cliente-servidor em Go.

- **Servidor**: REST API + WebSocket — gerencia jogos, turnos, upload/download de arquivos, notificações por e-mail
- **Cliente**: Desktop GUI (Wails v2) + modo headless — monitora pasta `.vlog`, sincroniza upload/download, notificações desktop

## Stack Técnica

| Camada | Tecnologia |
|--------|-----------|
| Linguagem | Go 1.23 |
| Desktop UI | [Wails v2](https://wails.io/) + HTML/CSS/JS vanilla |
| Banco de dados | SQLite (`modernc.org/sqlite`) ou PostgreSQL (`pgx`) |
| WebSocket | `gorilla/websocket` |
| File watcher | `fsnotify` |
| Notificações desktop | `beeep` |
| E-mail | `net/smtp` stdlib |
| Frontend assets | Embutidos via `//go:embed` |

## Estrutura do Projeto

```
cmd/
  client/           # Entry point do cliente (Wails GUI + headless)
    main.go           # Build: !headless → Wails GUI
    main_headless.go  # Build: headless → CLI puro
    wails_app.go      # Bridge Go ↔ frontend JS
    wails.json        # Config do Wails
    frontend/
      dist/           # Frontend SPA (HTML, CSS, JS vanilla)
      wailsjs/        # Wails auto-generated bindings
  server/
    main.go           # Entry point do servidor HTTP
internal/
  clientapp/         # Lógica de negócio do cliente (CRUD via HTTP)
  notify/            # Wrapper de notificações desktop (beeep)
  server/            # Handlers HTTP, store (SQL), e-mail
  sync/              # (vazio — reservado para lógica de sincronização)
  syncengine/        # Engine principal: watcher + upload/download + WS + polling
  vlog/              # Leitura de metadados de arquivos .vlog (ZIP + XML)
  watcher/           # Monitor de sistema de arquivos (fsnotify + .vlog)
  ws/                # Hub WebSocket do servidor
pkg/
  config/            # Persistência de config do cliente (JSON em UserConfigDir)
  models/            # Modelos compartilhados (Game, Player, Turn, DTOs)
```

## Fluxo de Dados

```
Jogador A (host)                 Servidor                    Jogador B
     │                              │                           │
     ├─ POST /games (criar) ──────►│                           │
     │◄─ invite_token, client_token│                           │
     │                              │                           │
     │                              │◄── POST /join ────────────┤
     │                              │──► WS: player_joined ────►│
     │                              │                           │
     ├─ POST /games/{id}/start ───►│                           │
     │                              │──► WS: turn_changed ─────►│
     │                              │──► e-mail notification ──►│
     │                              │                           │
     │  (A joga no Vassal)         │                           │
     ├─ watcher detecta novo .vlog │                           │
     ├─ POST /games/{id}/upload ──►│                           │
     │                              │──► WS: turn_changed ─────►│
     │                              │──► e-mail notification ──►│
     │                              │                           │
     │                              │◄── GET /download ─────────┤
     │                              │──► .vlog file ───────────►│
     │                              │    (B recebe notificação) │
     │                              │    (B joga no Vassal)     │
     │                              │                           │
     │◄── WS: turn_changed ────────│◄── POST /upload ──────────┤
     │◄── GET /download ───────────│                           │
     │  (ciclo se repete)          │                           │
```

## Como Rodar

### Desenvolvimento

```bash
# Servidor
go run ./cmd/server

# Cliente headless
go run -tags headless ./cmd/client --watch /caminho/vassal --server http://localhost:8080

# Cliente GUI (requer Wails CLI + webkit2gtk-4.1 no Arch/CachyOS)
./scripts/dev-gui.sh
```

### Build

```bash
./scripts/build.sh
# ou cross-compile:
GOOS=windows GOARCH=amd64 ./scripts/build.sh
```

### Testes

```bash
go test ./...
```

### PostgreSQL (opcional)

```bash
docker compose up -d
DATABASE_DRIVER=postgres \
DATABASE_DSN="postgres://vassal:vassal@localhost:5432/vassalvlogsync?sslmode=disable" \
go run ./cmd/server
```

## Variáveis de Ambiente (Servidor)

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `ADDR` | `:8080` | Endereço HTTP |
| `DATABASE_DRIVER` | `sqlite` | `sqlite` ou `postgres` |
| `DATABASE_DSN` | `file:./data/vassalvlogsync.db?_pragma=foreign_keys(1)` | DSN do banco |
| `DATA_DIR` | `./data/vlogs` | Armazenamento de `.vlog` |
| `BASE_URL` | `http://localhost:8080` | URL base para links de convite |
| `SMTP_HOST` | — | Servidor SMTP (opcional) |
| `SMTP_PORT` | `587` | Porta SMTP |
| `SMTP_USER` | — | Usuário SMTP |
| `SMTP_PASSWORD` | — | Senha SMTP |
| `SMTP_FROM` | =SMTP_USER | Remetente |

## Estado Atual do Projeto

### ✅ Funcionalidades Implementadas

- [x] Servidor REST com CRUD de jogos e turnos
- [x] Autenticação por token (bearer) para jogadores
- [x] WebSocket para eventos em tempo real (turn_changed, player_joined)
- [x] Upload/download de arquivos .vlog multipart
- [x] Leitura de `dateSaved` de arquivos .vlog (ZIP + XML)
- [x] Monitor de sistema de arquivos com debounce (fsnotify)
- [x] Cliente Wails GUI com 4 abas (Partida ativa, Criar, Entrar, Config)
- [x] Cliente headless (CLI)
- [x] Notificações desktop (beeep)
- [x] Notificações por e-mail (SMTP)
- [x] Persistência de config do cliente (JSON)
- [x] Suporte a SQLite e PostgreSQL
- [x] CI/CD (GitHub Actions cross-platform)
- [x] Turn order reordenável via API (`PATCH /games/{id}/turn-order`)
- [x] Lobby com status antes de iniciar partida
- [x] Docker Compose para PostgreSQL local

### 🚧 Pendências / Melhorias Futuras

- [x] Reordenação de turnos na UI (drag-and-drop no lobby)
- [ ] Tray icon (minimizar para bandeja) — `HideWindowOnClose: true` já configurado mas sem ícone
- [x] Suporte a reconexão robusta do WebSocket com backoff exponencial + jitter + ping/pong keepalive
- [x] Rate limiting no servidor
- [x] Exclusão/abandono de partida
- [ ] Histórico de turnos visualizável
- [ ] Indicador de progresso no upload/download
- [x] Suporte a múltiplas partidas simultâneas no cliente
- [ ] Seeds/arquivos auxiliares do Vassal (não apenas .vlog)
- [ ] Logs de auditoria no servidor
- [x] Health check do WebSocket no frontend (via ping/pong keepalive)
- [ ] Suporte a Docker do servidor — ver `PLANO_DEPLOY.md` e `GUIA_DEPLOY.md` (arquitetura + passo a passo)

## Convenções de Código

- Go: idiomatic, sem comentários desnecessários, `log.Fatal` para erros fatais, `log.Printf` para warnings
- Erros: variáveis `Err*` no package, `fmt.Errorf` com `%w` para wrapping
- SQL: suporte dual SQLite/PostgreSQL via `s.q()` que converte `?` → `$1`, `$2`, etc.
- Testes: Go stdlib testing, sem frameworks externos
- Frontend: vanilla JS com módulos ES, sem frameworks, classes CSS BEM-like
- Nomes de endpoints: RESTful com path params (`{id}`), não query params
- Arquivos: snake_case para Go, camelCase para JS/HTML

## Commands

```bash
# Testar tudo
go test ./...

# Build servidor
go build -o dist/vassal-vlog-sync-server ./cmd/server

# Build cliente headless
go build -tags headless -o dist/vassal-vlog-sync-headless ./cmd/client

# Rodar servidor com SQLite
go run ./cmd/server

# Rodar servidor com PostgreSQL
docker compose up -d
DATABASE_DRIVER=postgres DATABASE_DSN="postgres://vassal:vassal@localhost:5432/vassalvlogsync?sslmode=disable" go run ./cmd/server

# Dev GUI (Wails)
./scripts/dev-gui.sh
```
