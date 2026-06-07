# PLANO_DEPLOY.md — Vassal vLog Sync Server

## Visão Geral

Este plano transforma o servidor atual (binário local, SQLite, arquivos em disco) em uma aplicação cloud-native capaz de servir milhares de jogadores simultâneos com alta disponibilidade.

## Estimativa de Carga

| Métrica | Valor |
|---------|-------|
| Jogadores ativos/dia | 1.000 — 5.000 |
| Partidas simultâneas | 100 — 500 |
| Conexões WebSocket | 500 — 2.000 |
| Uploads/downloads/dia | 1.000 — 10.000 |
| Tamanho médio .vlog | 5 — 20 MB |
| Armazenamento/mês | 50 — 200 GB |

---

## Arquitetura Final

```
                      ┌──────────────────────────┐
                      │   Cloudflare / DNS        │
                      │   (CDN + proteção DDoS)   │
                      └──────────┬───────────────┘
                                 │
                      ┌──────────▼───────────────┐
                      │   Caddy (proxy reverso)   │
                      │   HTTPS (Let's Encrypt)   │
                      │   WebSocket passthrough   │
                      └──────────┬───────────────┘
                                 │
             ┌───────────────────┼───────────────────┐
             │                   │                   │
    ┌────────▼────────┐ ┌───────▼────────┐ ┌────────▼────────┐
    │  Server #1       │ │  Server #2     │ │  Server #N      │
    │  Go :8080        │ │  Go :8080      │ │  Go :8080       │
    │  REST + WS       │ │  REST + WS     │ │  REST + WS      │
    └────────┬─────────┘ └───────┬────────┘ └────────┬────────┘
             │                   │                    │
             └───────────────────┼────────────────────┘
                                 │
             ┌───────────────────┼────────────────────┐
             │                   │                    │
    ┌────────▼────────┐ ┌───────▼────────┐ ┌─────────▼────────┐
    │  PostgreSQL      │ │  Redis          │ │  S3-Compatible   │
    │  (pgxpool)       │ │  rate limiting  │ │  vlog storage    │
    │  migrations      │ │  WS pub/sub     │ │  MinIO/S3/R2     │
    └──────────────────┘ └────────────────┘ └──────────────────┘
```

### Componentes e responsabilidades

| Componente | Responsabilidade | Por que não o atual |
|-----------|-----------------|---------------------|
| **PostgreSQL** | Jogos, jogadores, turnos, metadados | SQLite tem concorrência limitada, não escala horizontal |
| **Redis** | Rate limiting compartilhado + WS pub/sub cross-instance | Rate limiter em memória não funciona com múltiplas instâncias; broadcast WS só alcança clients locais |
| **S3-Compatible** | Armazenamento de arquivos .vlog | Disco local não é acessível por múltiplas instâncias |
| **Caddy** | HTTPS automático, proxy reverso, WebSocket | Necessário para expor servidor à internet com TLS |
| **Docker Compose** | Orquestração de todos serviços | Build e deploy reprodutíveis |

---

## Dependências Novas (Go)

```
github.com/redis/go-redis/v9     # Redis client
github.com/minio/minio-go/v7     # S3/MinIO client
github.com/jackc/pgx/v5/pgxpool  # Connection pool PostgreSQL
```

---

## Arquivos — Lista Completa

### Arquivos Novos (14)

| # | Arquivo | Fase | Descrição |
|---|---------|------|-----------|
| N1 | `internal/server/blobstore.go` | 1 | Interface `BlobStore` + factory por driver |
| N2 | `internal/server/blobstore_local.go` | 1 | `LocalBlobStore` — salva em `./data/vlogs/` |
| N3 | `internal/server/blobstore_s3.go` | 1 | `S3BlobStore` — S3/MinIO/R2/Wasabi |
| N4 | `migrations/001_init.sql` | 1 | Schema SQL extraído do `migrate()` atual |
| N5 | `internal/server/migrate.go` | 1 | Runner que aplica arquivos `.sql` em ordem numérica |
| N6 | `internal/server/ratelimit_memory.go` | 2 | Código atual extraído para implementar interface |
| N7 | `internal/server/ratelimit_redis.go` | 2 | Token bucket via Redis (`INCR` + `EXPIRE` com janela deslizante) |
| N8 | `internal/server/health.go` | 2 | `GET /health` — ping DB + Redis + Storage + versão |
| N9 | `internal/ws/redis.go` | 3 | Pub/Sub adapter: `Publish` eventos + `Subscribe` em goroutine |
| N10 | `Dockerfile` | 1 | Multi-stage: `golang:1.23-alpine` → `distroless/static` |
| N11 | `Caddyfile` | 1 | Proxy reverso HTTP→HTTPS, WebSocket passthru |
| N12 | `.env.example` | 1 | Template com todas variáveis de ambiente documentadas |
| N13 | `PLANO_DEPLOY.md` | — | Este arquivo |

### Arquivos Modificados (5)

| # | Arquivo | Fase | Mudanças |
|---|---------|------|----------|
| M1 | `docker-compose.yml` | 1 | Reescrever: server + postgres + redis + caddy + minio(dev) |
| M2 | `internal/server/ratelimit.go` | 2 | Extrai rate limiter atual → interface `RateLimiter` + factory |
| M3 | `internal/server/store.go` | 2 | `pgxpool` para PostgreSQL prod, mantém `sql.DB` para SQLite dev; `UploadTurn` e `LatestVlogPath` usam `BlobStore` |
| M4 | `internal/ws/hub.go` | 3 | Hub ganha campo `redis`, subscribe no startup, broadcast publica no Redis |
| M5 | `cmd/server/main.go` | 2 | Novos drivers (`STORAGE_DRIVER`, `RATE_LIMIT_DRIVER`), graceful shutdown com `signal.NotifyContext` |

---

## Interfaces de Código

### BlobStore (`internal/server/blobstore.go`)

```go
type BlobStore interface {
    Put(ctx context.Context, key string, r io.Reader, size int64) error
    Get(ctx context.Context, key string) (io.ReadCloser, error)
}

func NewBlobStore(driver string) (BlobStore, error) {
    switch driver {
    case "s3":
        return newS3Store()
    default:
        return &LocalBlobStore{dataDir: os.Getenv("DATA_DIR")}, nil
    }
}
```

**Variáveis de ambiente:**
- `STORAGE_DRIVER` — `local` (padrão) ou `s3`
- `S3_ENDPOINT` — endpoint S3 (ex: `https://s3.amazonaws.com`)
- `S3_ACCESS_KEY`, `S3_SECRET_KEY` — credenciais
- `S3_BUCKET` — nome do bucket
- `S3_USE_SSL` — `true` (padrão)

### RateLimiter (`internal/server/ratelimit.go`)

```go
type RateLimiter interface {
    Allow(ctx context.Context, key string) bool
    Middleware(next http.Handler) http.Handler
}

func NewRateLimiter(driver string) RateLimiter {
    switch driver {
    case "redis":
        return newRedisRateLimiter()
    default:
        return newMemoryRateLimiter(30, 60)
    }
}
```

**Variáveis de ambiente:**
- `RATE_LIMIT_DRIVER` — `memory` (padrão) ou `redis`
- `RATE_LIMIT_RATE` — req/s (padrão: `30`)
- `RATE_LIMIT_BURST` — burst máximo (padrão: `60`)

### WebSocket Hub com Redis (`internal/ws/hub.go` + `redis.go`)

```go
type Hub struct {
    mu         sync.RWMutex
    clients    map[*client]struct{}
    redis      *redis.Client     // nil se Redis não configurado
    subCancel  context.CancelFunc
}

func (h *Hub) BroadcastTurnChanged(gameID string, ...) {
    // 1. Broadcast local (clientes desta instância)
    h.broadcastLocal(event)
    // 2. Publica no Redis para outras instâncias
    if h.redis != nil {
        data, _ := json.Marshal(event)
        h.redis.Publish(ctx, "ws:broadcast:"+gameID, data)
    }
}

func (h *Hub) listenRedis(ctx context.Context) {
    if h.redis == nil { return }
    pubsub := h.redis.PSubscribe(ctx, "ws:broadcast:*")
    for msg := range pubsub.Channel() {
        var event models.WSEvent
        json.Unmarshal([]byte(msg.Payload), &event)
        h.broadcastLocal(event)
    }
}
```

**Variáveis de ambiente:**
- `REDIS_ADDR` — endereço Redis (ex: `redis:6379`)
- `REDIS_PASSWORD` — senha (opcional)
- `REDIS_DB` — database number (padrão: `0`)

---

## Migrations (`migrations/`)

### `001_init.sql`

Schema extraído do método `migrate()` atual em `internal/server/store.go`, adaptado para PostgreSQL:

```sql
-- +migrate Up
CREATE TABLE IF NOT EXISTS games (
    id TEXT PRIMARY KEY,
    invite_token TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    vassal_module TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'lobby',
    current_turn_index INTEGER NOT NULL DEFAULT 0,
    host_player_id TEXT,
    created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS players (
    id TEXT PRIMARY KEY,
    game_id TEXT NOT NULL REFERENCES games(id),
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    client_token TEXT UNIQUE NOT NULL,
    turn_order INTEGER NOT NULL DEFAULT -1
);

CREATE TABLE IF NOT EXISTS turns (
    id TEXT PRIMARY KEY,
    game_id TEXT NOT NULL REFERENCES games(id),
    player_id TEXT NOT NULL REFERENCES players(id),
    vlog_path TEXT NOT NULL,
    date_saved TEXT NOT NULL,
    uploaded_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_players_game ON players(game_id);
CREATE INDEX IF NOT EXISTS idx_players_token ON players(client_token);
CREATE INDEX IF NOT EXISTS idx_games_invite ON games(invite_token);
```

### Runner (`internal/server/migrate.go`)

```go
func runMigrations(db *sql.DB, driver string) error {
    files, _ := filepath.Glob("migrations/*.sql")
    sort.Strings(files)
    for _, f := range files {
        sql, _ := os.ReadFile(f)
        q := adaptDialect(string(sql), driver) // ? → $1 p/ PostgreSQL
        db.Exec(q)
    }
    return nil
}
```

---

## Deploy — Comando Final

```bash
# Pré-requisitos no VPS
apt install docker.io docker-compose-v2

# Clonar e configurar
git clone <repo>
cd "Vassal vLog Sync"
cp .env.example .env
# editar .env com domínio, S3, SMTP, etc.

# Deploy
docker compose up -d

# Caddy obtém HTTPS automaticamente
# Servidor online em: https://vassal.seuservidor.com
```

### docker-compose.yml (visão geral)

```yaml
services:
  server:
    build: .
    env_file: .env
    depends_on: [postgres, redis]
    restart: unless-stopped
    # escala: docker compose up -d --scale server=3

  caddy:
    image: caddy:2-alpine
    ports: ["80:80", "443:443"]
    volumes: ["./Caddyfile:/etc/caddy/Caddyfile"]

  postgres:
    image: postgres:16-alpine
    volumes: ["pgdata:/var/lib/postgresql/data"]
    # produção: usar managed (RDS, DigitalOcean, etc.)

  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes

  minio:  # apenas dev local
    image: minio/minio
    profiles: ["dev"]
```

---

## Fases de Implementação

### Fase 1 — Containerização base

| Ordem | Arquivo(s) | Verificação |
|-------|-----------|-------------|
| 1 | `internal/server/blobstore.go` | `go build ./cmd/server` |
| 2 | `internal/server/blobstore_local.go` | `go test ./internal/server/` |
| 3 | `internal/server/blobstore_s3.go` | testa criar bucket vazio no MinIO |
| 4 | `migrations/001_init.sql` | validar SQL manualmente |
| 5 | `internal/server/migrate.go` | `go test` com SQLite + PostgreSQL |
| 6 | `internal/server/store.go` (BlobStore) | `go test` — upload/download via interface |
| 7 | `.env.example` | revisão de todas variáveis |
| 8 | `Dockerfile` | `docker build -t vassal-server .` |
| 9 | `Caddyfile` | validar sintaxe |
| 10 | `docker-compose.yml` | `docker compose up -d` em dev |

**Checkpoint**: servidor rodando em container, PostgreSQL, upload/download via blob store.

### Fase 2 — Single-instance produção

| Ordem | Arquivo(s) | Verificação |
|-------|-----------|-------------|
| 11 | `internal/server/ratelimit_memory.go` | `go build` |
| 12 | `internal/server/ratelimit_redis.go` | testar Redis local |
| 13 | `internal/server/ratelimit.go` (refactor) | `go test` ambos drivers |
| 14 | `internal/server/store.go` (pgxpool) | `go test` com PostgreSQL |
| 15 | `internal/server/health.go` | `curl /health` retorna JSON |
| 16 | `cmd/server/main.go` (graceful shutdown) | `Ctrl+C` faz cleanup |

**Checkpoint**: rate limit distribuído, pool de conexões, health check, shutdown limpo.

### Fase 3 — Escala horizontal

| Ordem | Arquivo(s) | Verificação |
|-------|-----------|-------------|
| 17 | `internal/ws/redis.go` | testar pub/sub manual |
| 18 | `internal/ws/hub.go` (modificar) | `docker compose up --scale server=2` |

**Checkpoint final**: duas instâncias do servidor, jogador A conectado na #1, jogador B na #2 — evento `turn_changed` chega em ambos via Redis pub/sub.

---

## Variáveis de Ambiente Completas

```bash
# ── Servidor ────────────────────────────────────
ADDR=:8080
BASE_URL=https://vassal.seuservidor.com

# ── Database ────────────────────────────────────
DATABASE_DRIVER=postgres          # postgres | sqlite
DATABASE_DSN=postgres://user:pass@postgres:5432/vassalvlogsync?sslmode=disable

# ── Storage ─────────────────────────────────────
STORAGE_DRIVER=s3                 # local | s3
DATA_DIR=./data/vlogs             # apenas se STORAGE_DRIVER=local
S3_ENDPOINT=https://s3.amazonaws.com
S3_ACCESS_KEY=AKIA...
S3_SECRET_KEY=...
S3_BUCKET=vassal-vlogs
S3_USE_SSL=true

# ── Redis ───────────────────────────────────────
REDIS_ADDR=redis:6379
REDIS_PASSWORD=                   # opcional
REDIS_DB=0
RATE_LIMIT_DRIVER=redis           # memory | redis
RATE_LIMIT_RATE=30                # requisições/segundo
RATE_LIMIT_BURST=60               # burst máximo

# ── E-mail (SMTP) ───────────────────────────────
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=app@dominio.com
SMTP_PASSWORD=xxxx                # App Password no Gmail
SMTP_FROM=app@dominio.com
```

---

## Notas de Produção

- **PostgreSQL**: em produção, considere usar um serviço gerenciado (RDS, DigitalOcean Managed DB, Supabase) em vez do container do compose. Configure backups automáticos.
- **S3**: Wasabi e Backblaze B2 são alternativas mais baratas que AWS S3, compatíveis com a API S3.
- **Redis**: em produção, configure `--requirepass` e use `REDIS_PASSWORD`.
- **Caddy**: certificados SSL renovados automaticamente. Precisa de porta 80 e 443 expostas.
- **Escala**: `docker compose up -d --scale server=3` sobe 3 instâncias. Caddy faz round-robin. Para WebSocket sticky sessions em load balancers externos, use `ip_hash` ou header `Upgrade`.
