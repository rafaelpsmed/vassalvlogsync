# Vassal vLog Sync

Sincroniza arquivos `.vlog` do [Vassal Engine](https://www.vassalengine.org/) entre jogadores automaticamente.

## Componentes

- **Servidor** (`cmd/server`) — API REST + WebSocket, lobby, turnos, upload/download, e-mail
- **Cliente** (`cmd/client`) — monitora pasta `.vlog`, sincroniza em background, UI Wails
- **Cliente headless** — `go build -tags headless ./cmd/client`

## Desenvolvimento local

```bash
# Servidor (SQLite por padrão)
go run ./cmd/server

# Cliente headless
go run -tags headless ./cmd/client --watch /caminho/vassal --server http://localhost:8080

# Cliente com UI (requer Wails + dependências do sistema)
# Arch/CachyOS (WebKit 4.1):
sudo pacman -S webkit2gtk-4.1 pkg-config gtk3

# Instalar Wails CLI (se ainda não tiver):
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Opção 1 — script (funciona mesmo se ~/go/bin não estiver no PATH):
./scripts/dev-gui.sh

# Opção 2 — manual (fish/bash: adicione ~/go/bin ao PATH):
fish -c 'fish_add_path ~/go/bin; cd cmd/client; and wails dev'
# ou em bash/zsh:
export PATH="$HOME/go/bin:$PATH" && cd cmd/client && wails dev -tags webkit2_41
```

A tag `webkit2_41` já está configurada em `cmd/client/wails.json` para distros que não têm mais o pacote `webkit2gtk-4.0`.

Frontend em `cmd/client/frontend/dist/`.

## PostgreSQL

```bash
docker compose up -d
export DATABASE_DRIVER=postgres
export DATABASE_DSN="postgres://vassal:vassal@localhost:5432/vassalvlogsync?sslmode=disable"
go run ./cmd/server
```

## Variáveis de ambiente (servidor)

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `ADDR` | `:8080` | Endereço HTTP |
| `DATABASE_DRIVER` | `sqlite` | `sqlite` ou `postgres` |
| `DATABASE_DSN` | `file:./data/vassalvlogsync.db` | DSN do banco |
| `DATA_DIR` | `./data/vlogs` | Armazenamento de `.vlog` |
| `BASE_URL` | `http://localhost:8080` | URL base para links de convite |
| `SMTP_*` | — | Configuração de e-mail (opcional) |

## Build multiplataforma

```bash
chmod +x scripts/build.sh
./scripts/build.sh
# ou
GOOS=windows GOARCH=amd64 ./scripts/build.sh
```

Frontend embarcado em `cmd/client/frontend/dist`.

## Fluxo

1. Host: **Criar Partida** → nome + módulo Vassal → compartilha link no Discord
2. Jogadores: **Entrar em Partida** → colam link/token
3. Host inicia a partida quando todos entraram
4. Ao salvar `.vlog` no Vassal, o arquivo sobe automaticamente; o próximo jogador recebe notificação e download
