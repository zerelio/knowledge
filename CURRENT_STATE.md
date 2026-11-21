# CURRENT_STATE.md — Zerelio Platform

Last updated: 2026-11-21 (Session 002)

## Status: Milestones 0.2 + 0.3 (partial) complete

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
| API — HTTP handlers | ✅ Complete | 16 handlers (refresh, neighborhood, cypher), 8 tests |
| Discovery — Plugin interface | ✅ Complete | Channel-based streaming |
| Discovery — Orchestrator | ✅ Complete | Updates scope LastRun; async AGE sync after run |
| Discovery — Plugin loader | ✅ Complete | ZERELIO_PLUGIN_DIR auto-discovery at startup |
| Graph — Neighborhood query | ✅ Complete | Recursive CTE, works without AGE |
| Graph — AGE sync | ✅ Complete | SyncResource/SyncBatch via Cypher MERGE |
| Graph — Cypher endpoint | ✅ Complete | POST /api/v1/graph/cypher (raw AGE Cypher) |
| DB — Initial schema migration | ✅ Complete | 001_initial_schema.up.sql |
| DB — Graph extensions migration | ✅ Complete | 002_graph_extensions.up.sql (AGE + pgvector + FTS) |
| Server entrypoint | ✅ Complete | Graceful shutdown, JWT key loading |
| Docker Compose | ✅ Complete | Fixed paths; postgres (AGE) + zerelio |
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
internal/api        — 8 tests (register, login, refresh, CRUD, scope, neighborhood)
internal/discovery  — 2 tests (run scope, unknown plugin)
internal/storage    — integration tests (build tag: integration, needs Docker)
Total: 19 unit tests, 0 races, all passing
```

## What is NOT yet implemented

### High priority (Milestone 0.3 remainder)

- [ ] Search module (hybrid BM25 + pgvector + graph proximity) — `GET /api/v1/search?q=...`
- [ ] pgvector: vector column on resources table, embedding storage

### Next milestone (0.4)

- [ ] Docs/runbook generation module (LLM context assembly)
- [ ] MCP server module — search_resources, get_resource, get_relationships, query_graph tools

### Medium priority

- [ ] Rate limiting middleware (token bucket per IP + per user)
- [ ] Audit logging (`audit_events` table)
- [ ] Pagination cursor (currently offset-based)

### Future (Zerelio Security)

- [ ] `security/` module scaffold
- [ ] Commercial license module
- [ ] RBAC policy engine
- [ ] Threat intelligence enrichment

## Known gaps

1. Discovery plugins exist as standalone gRPC binaries (24 repos); end-to-end subprocess run untested (requires ZERELIO_PLUGIN_DIR + a built binary).
2. DiscoveryScope.CreatedAt/UpdatedAt: zero values in Go type (data persisted in DB, just not read back).
3. AGE graph sync: tested structurally but not against a live AGE instance (integration tests pending).
