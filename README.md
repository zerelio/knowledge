# Zerelio Knowledge

Self-hosted infrastructure knowledge graph platform. Discovers, indexes, and understands your infrastructure — then lets you query it in natural language.

[![CI](https://github.com/zerelio/knowledge/actions/workflows/ci.yml/badge.svg)](https://github.com/zerelio/knowledge/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## What it does

Zerelio connects to your cloud accounts, SaaS tools, and infrastructure APIs, builds a live knowledge graph of everything it finds, and exposes that graph through REST, MCP, and natural language search.

- **Discovery** — 22+ plugins: AWS, Kubernetes, GitHub, Slack, Grafana, PagerDuty, DNS, and more
- **Graph** — PostgreSQL + Apache AGE; resources and their relationships as a queryable graph
- **Search** — BM25 + pgvector + graph proximity with RRF fusion and MMR re-ranking
- **AI** — LLM-generated runbooks and documentation; MCP server for Claude/Cursor/Copilot
- **Compliance** — built-in policy rules evaluated after every discovery run
- **Webhooks** — HMAC-signed event delivery on resource changes
- **Console** — Next.js management UI at [zerelio/console](https://github.com/zerelio/console)

## Quick start

**Docker Compose:**
```bash
curl -fsSL https://raw.githubusercontent.com/zerelio/knowledge/main/docker-compose.yml -o docker-compose.yml
ZERELIO_JWT_SECRET=$(openssl rand -hex 32) docker compose up -d
```

**Helm:**
```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm dependency build helm/zerelio
helm install zerelio ./helm/zerelio \
  --set config.jwtSecret=$(openssl rand -hex 32) \
  --set postgresql.auth.postgresPassword=$(openssl rand -hex 24)
```

**Docker image:** `ghcr.io/zerelio/knowledge:latest`

See [OPERATIONS.md](OPERATIONS.md) for full configuration reference.

## Configuration

| Variable | Required | Description |
|---|---|---|
| `ZERELIO_DATABASE_URL` | Yes | PostgreSQL DSN with pgvector + AGE extensions |
| `ZERELIO_JWT_SECRET` | Yes | Min 32 bytes; used to sign Ed25519 JWT keys |
| `ZERELIO_LLM_PROVIDER` | No | `openai`, `anthropic`, or `ollama` |
| `ZERELIO_LLM_API_KEY` | No | API key for the LLM provider |
| `ZERELIO_OIDC_ISSUER` | No | OIDC issuer URL for SSO (Google, Okta, etc.) |
| `ZERELIO_SCHEDULER_ENABLED` | No | `true` to run discovery on scope schedules |

## Architecture

Single Go binary. No sidecar processes. PostgreSQL is the only required dependency (pgvector + Apache AGE extensions for vector search and graph queries).

```
┌─────────────────────────────────────┐
│           Zerelio Knowledge         │
│  REST API  │  MCP Server  │  Auth   │
│            │              │         │
│  Discovery Orchestrator             │
│  ├── Plugin SDK (subprocess)        │
│  ├── Change Detector                │
│  └── Compliance Evaluator           │
│                                     │
│  Search: BM25 + pgvector + Graph    │
│  LLM: OpenAI / Anthropic / Ollama   │
└──────────────┬──────────────────────┘
               │
        PostgreSQL + AGE + pgvector
```

## Plugins

Plugins are separate binaries that implement the [plugin-sdk](https://github.com/zerelio/plugin-sdk) interface. Each plugin discovers resources for one platform and streams them back to the orchestrator.

Available plugins: [plugin-aws](https://github.com/zerelio/plugin-aws) · [plugin-github](https://github.com/zerelio/plugin-github) · [plugin-slack](https://github.com/zerelio/plugin-slack) · [plugin-grafana](https://github.com/zerelio/plugin-grafana) · [plugin-pagerduty](https://github.com/zerelio/plugin-pagerduty) · [plugin-dns](https://github.com/zerelio/plugin-dns)

## License

Apache 2.0 — see [LICENSE](LICENSE).
