# Zerelio — Operations Guide

This document covers everything an operator needs to run Zerelio in production.

---

## 1. What runs

```
zerelio binary              — Go server, single process
  ├── API       :8080       — REST API + auth
  ├── MCP       :9090       — AI agent integration (optional)
  ├── Embedder              — background worker, 15-min ticker
  └── OTel spans            — trace every LLM call

PostgreSQL 16+              — the ONLY database
  ├── Apache AGE            — knowledge graph (Cypher queries)
  ├── pgvector              — vector similarity search
  └── Standard tables       — resources, users, audit, AI output

Plugin binaries             — 24 separate gRPC subprocesses (optional)
  └── loaded from ZERELIO_PLUGIN_DIR at startup

LLM API (external)          — OpenAI / Anthropic / local Ollama
  └── for embeddings + doc/runbook generation
```

**No Redis, no S3, no message queue.** All state lives in PostgreSQL.
Multiple `zerelio` processes can share one database safely — the server is stateless.

---

## 2. Environment variables

### Required in production

| Variable | Description |
|---|---|
| `ZERELIO_POSTGRES_DSN` | PostgreSQL DSN, e.g. `postgres://user:pass@host:5432/db?sslmode=require` |
| `ZERELIO_JWT_PRIVATE_KEY` | Path to Ed25519 private key PEM file |

### Optional

| Variable | Default | Description |
|---|---|---|
| `ZERELIO_ADDR` | `:8080` | HTTP listen address |
| `ZERELIO_LOG_LEVEL` | `info` | `debug` / `warn` / `error` |
| `ZERELIO_AUTH_DISABLED` | `false` | **Dev only.** Disables JWT enforcement |
| `ZERELIO_PLUGIN_DIR` | — | Directory of plugin binaries. Discovery is disabled if unset |
| `ZERELIO_MIGRATIONS_DIR` | (auto) | Path to `db/migrations/`. Auto-resolved in dev; set explicitly in containers |
| `ZERELIO_MCP_HTTP` | `false` | Enable MCP HTTP+SSE transport |
| `ZERELIO_MCP_PORT` | `9090` | MCP HTTP port |
| `ZERELIO_MCP_STDIO` | `false` | Enable MCP stdio transport |
| `ZERELIO_OTEL_ENABLED` | `false` | Enable OTel tracing (stdout exporter) |

### LLM (AI features)

| Variable | Default | Description |
|---|---|---|
| `ZERELIO_LLM_PROVIDER` | — | `openai` / `anthropic` / `ollama`. AI features disabled if unset |
| `ZERELIO_LLM_API_KEY` | — | API key (OpenAI / Anthropic). Not required for Ollama |
| `ZERELIO_LLM_MODEL` | `gpt-4o-mini` | Chat/completion model |
| `ZERELIO_EMBEDDING_MODEL` | `text-embedding-3-small` | Embedding model |
| `ZERELIO_LLM_BASE_URL` | — | Override API base URL (Ollama: `http://localhost:11434`) |

---

## 3. PostgreSQL requirements

### Extensions

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS age;    -- Apache AGE (graph queries)
CREATE EXTENSION IF NOT EXISTS vector; -- pgvector (embedding search)
```

Extensions are created automatically by migration 001 / 002 / 003.
AGE and pgvector failures are non-fatal — features degrade gracefully.

### Recommended image for development

The official `apache/age:latest` image has AGE but NOT pgvector.
Build a combined image:

```dockerfile
FROM apache/age:latest
RUN apt-get update && apt-get install -y postgresql-server-dev-16 build-essential git \
 && git clone --branch v0.7.0 https://github.com/pgvector/pgvector /tmp/pgvector \
 && cd /tmp/pgvector && make && make install \
 && rm -rf /tmp/pgvector
```

Or use `pgvector/pgvector:pg16` (pgvector only) and install AGE separately.

### Managed PostgreSQL (production)

| Provider | Notes |
|---|---|
| AWS RDS | Use `rds.force_ssl = 1`. AGE not supported — graph features degrade. Use Aurora PostgreSQL for extensions. |
| Google Cloud SQL | Supports pgvector. AGE requires AlloyDB or self-managed. |
| Azure Database for PostgreSQL | Flexible Server supports pgvector; AGE not supported. |
| Self-managed | Full extension support. Use Patroni for HA. |

> **Note:** If your managed PostgreSQL does not support AGE, the `graph.Setup()` call
> logs a warning and graph features are silently disabled. The rest of the platform
> (discovery, search, docs) continues to work.

---

## 4. Schema migrations

Migrations run **automatically on startup** via `golang-migrate`.
They are idempotent — safe to run multiple times.

```
db/migrations/
├── 001_initial_schema.up.sql   — core tables, RLS, indexes
├── 002_graph_extensions.up.sql — AGE + pgvector extensions, FTS index
├── 003_vector_embeddings.up.sql — resources.embedding vector(1536)
├── 004_audit_events.up.sql      — audit_events table
└── 005_generation_logs.up.sql   — generation_logs, feedback, documents
```

**Override migration path in containers:**
```bash
ZERELIO_MIGRATIONS_DIR=/app/migrations
```

Mount `db/migrations/` into `/app/migrations/` in your container.

---

## 5. First-run checklist

```bash
# 1. Generate Ed25519 key pair
openssl genpkey -algorithm ed25519 -out /etc/zerelio/jwt_private.pem

# 2. Set environment
export ZERELIO_POSTGRES_DSN="postgres://zerelio:secret@db:5432/zerelio?sslmode=require"
export ZERELIO_JWT_PRIVATE_KEY="/etc/zerelio/jwt_private.pem"
export ZERELIO_MIGRATIONS_DIR="/app/db/migrations"

# 3. Start (migrations run automatically)
./zerelio

# 4. Register the first admin user
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"your-strong-password","roles":["admin"]}'

# 5. Set up discovery plugins
export ZERELIO_PLUGIN_DIR="/opt/zerelio/plugins"
# Copy plugin binaries to ZERELIO_PLUGIN_DIR, restart.

# 6. Enable AI features (optional)
export ZERELIO_LLM_PROVIDER=openai
export ZERELIO_LLM_API_KEY=sk-...
```

---

## 6. Docker Compose (development)

```bash
# Start the stack (Postgres + Zerelio)
docker compose up -d

# Check logs
docker compose logs -f zerelio

# The server is at http://localhost:8080
# Auth is DISABLED in the compose file — do not use in production
```

> **Vector search:** The compose file uses `apache/age:latest` which lacks pgvector.
> Vector embeddings are stored but similarity search falls back to BM25 only.
> Build the combined image above for full functionality.

---

## 7. Kubernetes deployment (sketch)

```yaml
# Secret
apiVersion: v1
kind: Secret
metadata:
  name: zerelio-secrets
stringData:
  jwt_private.pem: |
    -----BEGIN PRIVATE KEY-----
    ...
  postgres_dsn: "postgres://zerelio:secret@postgres:5432/zerelio?sslmode=require"
  llm_api_key: "sk-..."

---
# Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zerelio
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: zerelio
        image: ghcr.io/zerelio/zerelio:latest
        env:
        - name: ZERELIO_POSTGRES_DSN
          valueFrom:
            secretKeyRef: { name: zerelio-secrets, key: postgres_dsn }
        - name: ZERELIO_JWT_PRIVATE_KEY
          value: /secrets/jwt_private.pem
        - name: ZERELIO_MIGRATIONS_DIR
          value: /app/db/migrations
        - name: ZERELIO_PLUGIN_DIR
          value: /plugins
        - name: ZERELIO_LLM_PROVIDER
          value: openai
        - name: ZERELIO_LLM_API_KEY
          valueFrom:
            secretKeyRef: { name: zerelio-secrets, key: llm_api_key }
        volumeMounts:
        - name: secrets
          mountPath: /secrets
          readOnly: true
        - name: plugins
          mountPath: /plugins
          readOnly: true
        - name: migrations
          mountPath: /app/db/migrations
          readOnly: true
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet: { path: /health, port: 8080 }
          initialDelaySeconds: 10
        readinessProbe:
          httpGet: { path: /health, port: 8080 }
          initialDelaySeconds: 5
      volumes:
      - name: secrets
        secret: { secretName: zerelio-secrets }
      - name: plugins
        # Distribute plugin binaries via ConfigMap, PVC, or init container
        emptyDir: {}
      - name: migrations
        configMap: { name: zerelio-migrations }
```

---

## 8. Plugin binaries

24 plugin binaries are in separate GitHub repos under `github.com/zerelio/plugin-*`.

**Build locally:**
```bash
mkdir -p /opt/zerelio/plugins
for plugin in aws k8s github slack gcp azure grafana jira; do
  git clone https://github.com/zerelio/plugin-${plugin}
  cd plugin-${plugin}
  CGO_ENABLED=0 go build -buildvcs=false -o /opt/zerelio/plugins/plugin-${plugin} .
  cd ..
done
```

**Plugin names** are derived by stripping the `plugin-` prefix:
`plugin-aws` → plugin name `aws`. Scope `plugin_name: "aws"` maps to this binary.

---

## 9. What degrades gracefully

If a dependency is missing, Zerelio continues with reduced functionality:

| Missing | Effect |
|---|---|
| AGE extension | Graph sync + Cypher endpoints return 503. Discovery, search, docs work. |
| pgvector extension | Vector column not created. BM25-only search. Everything else works. |
| `ZERELIO_LLM_PROVIDER` | Embedding pipeline disabled. Doc/runbook endpoints return 503. Search falls back to BM25+graph. |
| `ZERELIO_PLUGIN_DIR` | Discovery endpoints return "plugin not found". All read/query features work. |
| All missing | Platform starts with MemoryBackend, no persistence. Useful for smoke testing only. |

---

## 10. Backup and restore

```bash
# Full backup
pg_dump -Fc -h $DB_HOST -U $DB_USER $DB_NAME > zerelio-$(date +%Y%m%d).dump

# Restore to new database
createdb zerelio_restore
pg_restore -h $DB_HOST -U $DB_USER -d zerelio_restore zerelio-20260101.dump
# Then run zerelio against the restored DB — migrations will be a no-op
```

**What is in the backup:** all discovered resources, relationships, users, audit events, generated documents and their feedback, embeddings. Everything.

---

## 11. Known gaps (not yet production-ready)

- [ ] No TLS on the server itself (requires a reverse proxy: nginx, Caddy, AWS ALB)
- [ ] No Helm chart
- [ ] Plugin binaries must be built from source (`make plugins`) — no pre-built release artifacts yet
- [ ] `ZERELIO_JWT_PRIVATE_KEY` rotation requires a restart
