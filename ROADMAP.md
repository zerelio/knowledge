# ROADMAP.md — Zerelio Platform

## Milestone 0.1 — Foundation (complete)

The core server is functional with auth, storage interface, discovery orchestrator, and CI.

## Milestone 0.2 — Storage + Discovery (next)

**Goal**: End-to-end discovery working against a real PostgreSQL instance.

- [ ] `testcontainers-go` integration tests for PGBackend
- [ ] `POST /api/v1/refresh` — refresh token rotation
- [ ] Migrate AWS discovery plugin from source repo (adapt import paths)
- [ ] Migrate 5 more plugins (GCP, K8s, Azure, GitHub, Terraform)
- [ ] Scheduler — cron-based automatic scope runs
- [ ] Schema migration runner integration (golang-migrate)

## Milestone 0.3 — Graph + Search

**Goal**: Query the knowledge graph and search resources.

- [ ] Graph module: Apache AGE Cypher query layer
  - Neighborhood traversal (1-3 hops)
  - Path finding between resources
  - Subgraph export (filtered by type/provider)
- [ ] Search module: hybrid BM25 + pgvector + graph proximity
  - pgvector embedding storage (text-embedding-3-small)
  - BM25 via PostgreSQL `tsvector`
  - Three-signal fusion with RRF (Reciprocal Rank Fusion)
- [ ] `GET /api/v1/search?q=...` endpoint

## Milestone 0.4 — Docs + MCP

**Goal**: AI-generated runbooks and MCP server for LLM tool use.

- [ ] Docs module: LLM-based runbook generation
  - Resource context assembly
  - LightRAG dual-level retrieval
  - Runbook templating
- [ ] MCP server module (Model Context Protocol)
  - `search_resources` tool
  - `get_resource` tool
  - `get_relationships` tool
  - `query_graph` tool

## Milestone 0.5 — Hardening

**Goal**: Production-ready security and observability.

- [ ] Rate limiting (token bucket per IP + per user)
- [ ] Structured audit log (`audit_events` table)
- [ ] Prometheus metrics endpoint
- [ ] OpenTelemetry tracing
- [ ] `go vet`, `staticcheck`, `gosec` clean
- [ ] SBOM generation (CycloneDX)
- [ ] Remaining 16 plugin migrations

## Milestone 1.0 — Zerelio Security scaffold (complete)

**Goal**: Commercial product foundation as a separate bounded module.

- [x] `security/` module with go.mod (separate from platform)
- [x] RBAC policy engine — pure-Go enforcer, Rego policy spec, 6 tests
- [x] Threat intelligence enrichment — interface + NoopEnricher + MisconfigEnricher + CompositeEnricher, 5 tests
- [x] CI boundary enforcement — security/ cannot import zerelio/internal/
- ~~[ ] License validation module~~ — removed (see ADR-008: self-hosted OSS has no license server)
