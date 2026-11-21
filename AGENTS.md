# AGENTS.md — Zerelio Platform

This file documents conventions for AI agents (Claude Code and others) working in this repository.

## Repository layout

```
zerelio/
├── go.work                    # Go workspace — links all modules
├── knowledge/                 # Zerelio Knowledge (Apache-2.0, OSS)
│   ├── cmd/zerelio/main.go    # Server entry point
│   ├── internal/              # Private implementation — never imported by security/
│   │   ├── api/               # HTTP handlers (chi router)
│   │   ├── auth/              # JWT (Ed25519), argon2id passwords, RBAC middleware
│   │   ├── discovery/         # Plugin orchestrator
│   │   └── storage/           # Backend interface + MemoryBackend + PGBackend
│   ├── pkg/types/             # Public domain types
│   └── db/migrations/         # PostgreSQL schema (golang-migrate format)
├── security/                  # Zerelio Security (commercial) — not yet created
└── docs/                      # Architecture decisions, research, session logs
    ├── architecture/          # ADRs 001–007 + system-architecture.md
    ├── research/              # Scientific foundations, industry landscape
    └── phase-zero/            # Source inventory of original Knowledge Tree
```

## Critical constraints

1. **Never modify** `~/Desktop/GIT/knowledge-tree-git` — it is read-only source reference.
2. **Never push** to any remote repository.
3. **Never publish** packages, releases, or Docker images.
4. **Never provision** paid infrastructure.
5. **security/ must never import** `github.com/zerelio/zerelio/internal/` packages. The CI `license-boundary` job enforces this.

## Development commands

```bash
# Build
cd knowledge && GOWORK=off go build -buildvcs=false ./...

# Test (with race detector)
cd knowledge && GOWORK=off go test -buildvcs=false -race -count=1 ./...

# Vet
cd knowledge && go vet ./...

# Local dev server (auth disabled)
ZERELIO_AUTH_DISABLED=true go run ./cmd/zerelio

# Docker Compose (full stack)
docker compose up
```

## Architecture decisions

All ADRs are in `docs/architecture/`. Key decisions:

| ADR | Decision |
|-----|----------|
| 001 | Modular monolith (not microservices) |
| 002 | Go workspace monorepo with knowledge/ + security/ |
| 003 | Apache-2.0 for knowledge/, commercial for security/ |
| 004 | PostgreSQL + AGE + pgvector (single DB) |
| 005 | Apache-2.0 license — no GPL dependencies |
| 006 | Ed25519 JWT signing + argon2id password hashing |
| 007 | hashicorp/go-plugin (gRPC) for discovery plugins |

## Security invariants

- Server **refuses to start** unless `ZERELIO_JWT_PRIVATE_KEY` is set, unless `ZERELIO_AUTH_DISABLED=true` (dev only).
- Passwords hashed with **argon2id** (time=1, mem=64MB, threads=4, salt=16 random bytes). Never SHA-256.
- JWT signing uses **Ed25519** asymmetric keys. Only the auth module holds the private key.
- **Refresh tokens rotate** on every use (JTI stored in refresh_tokens table).
- PostgreSQL **RLS** enabled on all tenant-scoped tables.
