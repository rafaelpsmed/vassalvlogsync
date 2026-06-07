# Guia de Deploy — Vassal vLog Sync

Deploy completo do servidor em produção com Docker Compose, PostgreSQL, Redis, armazenamento S3 e HTTPS automático.

---

## 1. Pré-requisitos

- VPS Ubuntu 24.04 (Hetzner CX22 recomendado, ~€6/mês)
- Domínio apontando para o IP do VPS (`vassal.seuservidor.com`)
- Portas 80 e 443 liberadas no firewall

```bash
# Instalar Docker + Compose no VPS
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
# re-logar para aplicar grupo docker
```

---

## 2. Clonar e preparar

```bash
git clone <repo-url> vassal-vlog-sync
cd vassal-vlog-sync
cp .env.example .env
```

---

## 3. Configurar `.env`

Editar `nano .env`:

```bash
# ── Essencial ──
BASE_URL=https://vassal.seuservidor.com
DOMAIN=vassal.seuservidor.com

# ── Banco de dados ──
DATABASE_DRIVER=postgres
DATABASE_DSN=postgres://vassal:vassal@postgres:5432/vassalvlogsync?sslmode=disable

# ── Storage (.vlog files) ──
STORAGE_DRIVER=s3
S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
S3_ACCESS_KEY=abc123...
S3_SECRET_KEY=xyz789...
S3_BUCKET=vassal-vlogs

# ── Rate Limiting ──
RATE_LIMIT_DRIVER=redis
REDIS_ADDR=redis:6379

# ── E-mail (opcional) ──
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=seuemail@gmail.com
SMTP_PASSWORD=sua-senha-de-app
SMTP_FROM=seuemail@gmail.com
```

### Onde conseguir as credenciais S3

**Cloudflare R2 (recomendado)**: Crie um bucket `vassal-vlogs` no dashboard, gere Access Key e Secret Key. O endpoint é `https://<account-id>.r2.cloudflarestorage.com`.

**Backblaze B2**: Funciona com a API S3. Configure via Cloudflare Bandwidth Alliance para egress gratuito.

**AWS S3**: O endpoint padrão é `https://s3.amazonaws.com`.

---

## 4. Verificar Caddyfile

Já vem pronto — usa a variável `DOMAIN` do ambiente:

```
{$DOMAIN:localhost} {
    encode zstd gzip
    reverse_proxy server:8080
}
```

---

## 5. Subir os serviços

```bash
# Build da imagem do servidor (primeira vez: ~2 min)
docker compose build

# Subir tudo (servidor + Caddy + Redis + PostgreSQL)
docker compose --profile postgres up -d

# Verificar se está tudo rodando
docker compose ps
```

### Serviços que sobem

| Serviço | Porta | Descrição |
|---------|-------|-----------|
| caddy | 80, 443 | HTTPS automático, proxy reverso |
| server | 8080 (interna) | Aplicação Go |
| postgres | 5432 (interna) | Banco de dados |
| redis | 6379 (interna) | Rate limiting + WS pub/sub |

O MinIO (S3 local para dev) fica desativado em produção. Só sobe com `--profile dev`.

---

## 6. Verificar funcionamento

```bash
# Health check
curl https://vassal.seuservidor.com/health
# Resposta esperada:
# {"status":"ok","db":"ok","storage":"ok","go":"go1.25.0","version":"0.1.0"}

# Criar partida de teste
curl -X POST https://vassal.seuservidor.com/games \
  -H "Content-Type: application/json" \
  -d '{"name":"Teste","vassal_module":"CC Ancients","host_name":"Alice","host_email":"a@t.com"}'

# Página de convite no navegador
# https://vassal.seuservidor.com/join/<invite_token>
```

---

## 7. HTTPS

Caddy obtém certificados Let's Encrypt automaticamente na primeira requisição ao domínio. Renovação automática a cada ~60 dias.

**Importante**: O domínio precisa resolver para o IP do VPS **antes** de subir o Caddy. Se o DNS ainda não propagou, o Caddy não consegue validar e o certificado não é emitido.

---

## 8. Logs

```bash
# Servidor Go
docker compose logs -f server

# Caddy (inclui erros TLS)
docker compose logs caddy

# PostgreSQL
docker compose logs postgres
```

---

## 9. Atualizar

Quando houver código novo no repositório:

```bash
git pull
docker compose build server
docker compose up -d server
```

Dados (PostgreSQL, Redis, S3) não são afetados.

---

## 10. Backup

```bash
# Banco de dados
docker compose exec postgres pg_dump -U vassal vassalvlogsync > backup-$(date +%Y%m%d).sql

# .vlogs no S3/R2
# Configure versioning no bucket para proteção contra exclusão acidental
```

---

## 11. Escalar horizontalmente

Quando um servidor não for suficiente:

```bash
# Rodar 3 instâncias
docker compose up -d --scale server=3
```

O Redis pub/sub garante que eventos WebSocket cheguem em todas as instâncias. O rate limiting é compartilhado via Redis. Os arquivos estão no S3, acessível por qualquer instância. Caddy faz round-robin automático.

---

## 12. Troubleshooting

| Problema | Causa provável | Solução |
|----------|--------------|---------|
| HTTPS não funciona | DNS não propagou | `dig vassal.seuservidor.com` — ver se resolve pro IP |
| Erro de conexão no cliente | `BASE_URL` errado | `.env` → `BASE_URL=https://...` com https e domínio real |
| Upload falha | Credenciais S3 erradas | Ver `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, bucket existe? |
| `"storage":"unavailable"` no health | BlobStore não inicializou | Ver `STORAGE_DRIVER` e credenciais no `.env` |
| `"db":"error"` no health | PostgreSQL não subiu | `docker compose logs postgres` |
| E-mail não envia | SMTP não configurado | Deixar `SMTP_HOST` vazio desabilita; ver App Password do Gmail |
| Caddy erro "no such host" | `DOMAIN` não definido | Adicionar `DOMAIN=...` ao `.env` |

---

## 13. Comandos rápidos

```bash
docker compose ps                          # status dos serviços
docker compose logs -f server              # logs do servidor
docker compose restart server              # reiniciar após mudar .env
docker compose --profile postgres down     # parar tudo
docker compose --profile postgres up -d    # subir tudo
docker compose exec postgres psql -U vassal -d vassalvlogsync  # console SQL
```
