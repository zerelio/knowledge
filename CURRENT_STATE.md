# CURRENT_STATE.md — Zerelio Platform

Last updated: 2026-11-21 (Session 002)

## Status: Milestone 0.2 in progress — auth hardened, storage interface complete

### Core server

| Component | Status | Notes |
|-----------|--------|-------|
| Auth — argon2id password hashing | ✅ Complete | 5 tests |
| Auth — Ed25519 JWT service | ✅ Complete | JTI on access tokens, RefreshJTI exposed |
| Auth — RBAC middleware | ✅ Complete | Authenticator + RequireRole |
| Auth — Refresh token rotation | ✅ Complete | POST /api/v1/auth/refresh, single-use JTI |
| Storage — Backend interface | ✅ Complete | 32 methods (added GetUserByID, 4 refresh token methods) |
| Storage — MemoryBackend | ✅ Complete | Thread-safe, all 32 methods |
| Storage — PGBackend | ✅ Complete | All 32 methods; UpdateScope persists last_run |
| Storage — PGBackend integration tests | ✅ Complete | testcontainers-go, build tag: integration |
| API — HTTP handlers | ✅ Complete | 14 handlers (added HandleRefresh), 6 tests |
| Discovery — Plugin interface | ✅ Complete | Channel-based streaming |
| Discovery — Orchestrator | ✅ Complete | Updates scope LastRun + ResourceCount after run |
| DB — Initial schema migration | ✅ Complete | 001_initial_schema.up.sql |
| Server entrypoint | ✅ Complete | Graceful shutdown, JWT key loading |
| Docker Compose | ✅ Complete | postgres (AGE) + zerelio |
| Dockerfile | ✅ Complete | Distroless, CGO disabled |
| GitHub Actions CI | ✅ Complete | test + lint + boundary check + integration tests |

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
internal/auth       — 9 tests (password hashing, JWT, middleware)
internal/api        — 6 tests (register, login, refresh, CRUD resources)
internal/discovery  — 2 tests (run scope, unknown plugin)
internal/storage    — integration tests (build tag: integration, needs Docker)
Total: 17 unit tests, 0 races, all passing
```

## What is NOT yet implemented

### High priority (Milestone 0.2 remainder)

- [ ] Discovery plugin migration — wire one plugin (e.g. aws) end-to-end against the platform
- [ ] Discovery API: scope CRUD endpoint tests
- [ ] Graph module (AGE Cypher query layer)
- [ ] Search module (hybrid BM25 + pgvector + graph)
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

1. Discovery plugins exist as standalone gRPC binaries (24 repos) but none are registered with the orchestrator. A scope run always fails "plugin not found".
2. DiscoveryScope.CreatedAt/UpdatedAt are not read from DB (zero values in Go type). Low priority — data is persisted by Postgres default.
