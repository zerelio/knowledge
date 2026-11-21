# CURRENT_STATE.md — Zerelio Platform

Last updated: 2026-11-21 (Session 001)

## Status: Foundation complete — core server functional

The `knowledge/` module has a working, tested Go server.

## What is implemented

### Core server (`knowledge/`)

| Component | Status | Notes |
|-----------|--------|-------|
| Auth — argon2id password hashing | ✅ Complete | 5 tests |
| Auth — Ed25519 JWT service | ✅ Complete | 4 tests |
| Auth — RBAC middleware | ✅ Complete | Authenticator + RequireRole |
| Storage — Backend interface | ✅ Complete | 27 methods |
| Storage — MemoryBackend | ✅ Complete | Thread-safe, testing use |
| Storage — PGBackend | ✅ Complete | pgx/v5, type-aligned with schema |
| API — HTTP handlers | ✅ Complete | 13 handlers, 5 tests |
| Discovery — Plugin interface | ✅ Complete | Channel-based streaming |
| Discovery — Orchestrator | ✅ Complete | Async runs, 2 tests |
| DB — Initial schema migration | ✅ Complete | 001_initial_schema.up.sql |
| Server entrypoint | ✅ Complete | Graceful shutdown, JWT key loading |
| Docker Compose | ✅ Complete | postgres (AGE) + zerelio |
| Dockerfile | ✅ Complete | Distroless, CGO disabled |
| GitHub Actions CI | ✅ Complete | test + lint + boundary check |

### Documentation

| Document | Status |
|----------|--------|
| Phase Zero source inventory | ✅ Complete |
| Migration matrix (60 components) | ✅ Complete |
| Scientific foundations (30+ papers) | ✅ Complete |
| Industry landscape + competitors | ✅ Complete |
| System architecture diagram | ✅ Complete |
| ADRs 001–007 | ✅ Complete |
| Architectural self-review | ✅ Complete |

## Test coverage

```
knowledge/internal/auth       — 9 tests (password hashing, JWT, middleware)
knowledge/internal/api        — 5 tests (register, login, CRUD resources)
knowledge/internal/discovery  — 2 tests (run scope, unknown plugin)
Total: 16 tests, 0 races, all passing
```

## What is NOT yet implemented

### High priority (next session)

- [ ] PostgreSQL backend integration tests (testcontainers-go)
- [ ] Discovery plugin migration (22 plugins from source repo)
- [ ] Discovery API: scope CRUD endpoint tests
- [ ] Graph module (AGE Cypher query layer)
- [ ] Search module (hybrid BM25 + pgvector + graph)
- [ ] Refresh token rotation endpoint (`POST /api/v1/refresh`)
- [ ] go.work sum file (`go work sync`)

### Medium priority

- [ ] Docs/runbook generation module
- [ ] MCP (Model Context Protocol) server module
- [ ] Rate limiting middleware
- [ ] Audit logging
- [ ] Pagination cursor (currently offset-based)

### Future (Zerelio Security)

- [ ] `security/` module scaffold
- [ ] Commercial license module
- [ ] RBAC policy engine
- [ ] Threat intelligence enrichment

## Known gaps

1. `PGBackend` has no integration tests — it compiles but is untested against a real database.
2. Discovery plugins are not yet migrated from the source repo.
3. Refresh token storage (`refresh_tokens` table) exists in schema but `POST /refresh` endpoint is not wired.
4. `DiscoveryScope.CreatedAt/UpdatedAt` are in the DB schema but not in the Go type — the type uses `LastRun`.
