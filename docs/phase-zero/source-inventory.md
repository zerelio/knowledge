# Phase Zero: Knowledge Tree Source Inventory and Architecture Assessment

**Date:** 2026-11-21
**Assessor:** Autonomous Principal Architect
**Source Repository:** ~/Desktop/GIT/knowledge-tree-git (no git history -- flat copy)

---

## 1. Executive Summary

Knowledge Tree is an infrastructure discovery and auto-documentation platform. It discovers cloud resources across 22+ providers, stores them in an in-memory or PostgreSQL-backed knowledge graph, and generates documentation and runbooks via LLM integration.

The project is a **Go 1.26 workspace monorepo** containing 31 Go modules (8 core services + 22 plugins + 1 shared workflow repo), a React SPA frontend, three Next.js static marketing/docs sites, a Python RAG service, and co-located but unrelated projects (Trafalgar AI support platform).

**Overall assessment: 5-6/10.** The architecture is ambitious and well-conceived in many areas (plugin system, type isolation, storage abstraction), but implementation is approximately 60-70% complete with significant gaps in persistence, security, testing, and actual cloud discovery execution.

---

## 2. Repository Inventory

### 2.1 Go Modules (via go.work)

| Module | Lines of Go | Purpose | Assessment |
|--------|------------|---------|------------|
| knowledge-tree-types | ~1,200 | Shared domain types (zero deps) | **Excellent** -- clean, well-designed |
| knowledge-tree-core | ~56,300 | Core engine: storage, orchestrator, plugins, CLI, MCP, agent | **Good** -- 70-80% complete |
| knowledge-tree-sdk | ~2,500 | Plugin SDK: gRPC protocol, crypto, resilience | **Good** -- well-architected |
| knowledge-tree-api | ~8,000 | API gateway: 100+ endpoints, SSO, SCIM, compliance | **Fair** -- feature-rich but races/no persistence |
| knowledge-tree-auth | ~500 | JWT auth microservice | **Poor** -- data races, weak crypto, in-memory only |
| knowledge-tree-docs | ~2,000 | LLM doc/runbook generation | **Fair** -- works but in-memory store |
| knowledge-tree-graph | ~800 | Graph query service | **Poor** -- no write API, stubbed Cypher, in-memory |
| knowledge-tree-license | ~600 | Ed25519 license validation | **Good** -- clean, focused |
| 22 plugins | ~8,000 total | Cloud/SaaS discovery plugins | **Fair** -- architecture good, some incomplete |

### 2.2 Frontend / Web Projects

| Project | Stack | Purpose | Assessment |
|---------|-------|---------|------------|
| knowledge-tree-frontend | Vite + React 18 + TS + D3 | Product SPA | **Good** -- comprehensive but large components |
| knowledge-tree-web | Next.js 16 + React 19 | Marketing site | **Fair** -- bleeding-edge Next.js |
| knowledge-tree-web-apidocs | Next.js 16 + React 19 | API reference | **Fair** -- 150+ endpoints documented |
| knowledge-tree-web-docs | Next.js 16 + React 19 | Product docs | **Fair** -- comprehensive IA, many stub pages |
| knowledge-tree-marketing | Markdown | Brand/strategy content | **Good** for stage |

### 2.3 Other Components

| Component | Stack | Purpose | Assessment |
|-----------|-------|---------|------------|
| kt-rag | Python 3.10+ / LightRAG | RAG service with scoring | **Good** -- 43 tests, clean design |
| knowledge-tree-knowledge | Markdown | Project knowledge base | **Valuable** -- honest self-assessment |
| knowledge-tree-billing | Markdown only | Stripe billing (planned) | **Stub** -- no code |
| knowledge-tree-workflows | YAML | GitHub Actions CI/CD | **Good** -- mature pipeline |
| trafalgar/ | Node.js full-stack | AI support platform (separate product) | **Unrelated** to Zerelio |
| trafalgar-demo/ | Next.js static | Trafalgar demo | **Unrelated** |

### 2.4 Deployment Infrastructure

| Method | Location | Assessment |
|--------|----------|------------|
| Docker Compose (dev) | deploy/docker/ | **Works** -- LocalStack, 7 services |
| AWS EC2 single-instance | deploy/aws/ | **Works** -- PostgreSQL + AGE + Nginx |
| Helm chart (production) | deploy/helm-chart/ | **Good** -- HPA, PDB, NetworkPolicy, RBAC |

---

## 3. Architecture Analysis

### 3.1 System Architecture

```
Frontend (React :3000) --> API Gateway (:8080) --> Auth Service (:8083)
                                               --> Docs/LLM Service (:8081)
                                               --> Graph Service (:8082)
                                               --> License Service (:8084)
                                               --> RAG Service (:8085)
                       --> Core Library (imported directly by API)
                       --> Plugin Manager --> 22 plugins (gRPC, hashicorp/go-plugin)
```

The API gateway is the central aggregation point. It imports `knowledge-tree-core` as a Go library and proxies HTTP requests to the other four microservices. Plugins run as separate OS processes communicating via gRPC.

### 3.2 Data Flow

1. **Discovery:** User creates a DiscoveryScope with provider config -> API triggers discovery -> Plugin Manager loads plugin binary -> Plugin discovers resources via cloud API -> Resources streamed back as DiscoveryEvents via gRPC channel -> Stored in Backend (memory or PostgreSQL)
2. **Documentation:** User requests doc generation -> API proxies to Docs service -> Docs fetches graph context from Graph service -> LLM generates documentation -> Stored in memory
3. **RAG:** kt-rag seeds from API graph export -> LightRAG indexes resources -> User queries -> Hybrid retrieval + confidence scoring

### 3.3 Storage Architecture

**In-memory (default):** All services use `sync.Map` or plain Go maps. No persistence across restarts.

**PostgreSQL (production):**
- PostgreSQL 16 with Apache AGE extension (graph queries via Cypher)
- pgvector extension (vector similarity search, IVFFlat index, 1536 dimensions)
- 12 idempotent migrations with SHA-256 checksum integrity
- Circuit breaker pattern for connection resilience
- Backend interface with `MemoryBackend` and `PGBackend` implementations

### 3.4 Type System

The type system is well-designed with strict isolation:
- `knowledge-tree-types`: Zero external dependencies, ~100 resource types, 15 relationship types
- CI enforces that `knowledge-tree-graph` and `knowledge-tree-license` do NOT import `knowledge-tree-core`
- All plugins depend only on `knowledge-tree-sdk` which depends only on `knowledge-tree-types`

### 3.5 Plugin Architecture

The hashicorp/go-plugin framework provides:
- Out-of-process isolation (crash in plugin doesn't crash host)
- gRPC communication with protobuf-defined protocol
- Streaming discovery events via channel
- Plugin SDK with helpers, crypto, resilience (circuit breaker, retry)
- 22 plugins covering: AWS, Azure, GCP, K8s, DNS, GitHub, Grafana, Jira, Datadog, MongoDB, MySQL, PostgreSQL, vSphere, Cloudflare, PagerDuty, Confluence, Slack, ServiceNow, NewRelic, Prometheus, Elasticsearch, CodeGraph

---

## 4. What Works Well (Preserve)

### 4.1 Architecture Decisions to Keep
- **Zero-dep shared types module** -- excellent for cross-module compatibility
- **Plugin SDK with gRPC isolation** -- production-grade plugin architecture
- **Three-subsystem storage** (Graph/Metadata/Vector) -- well-designed for the domain
- **Type-enforced module boundaries** with CI verification
- **Resource/Relationship model** -- well thought-out for multi-cloud infrastructure
- **Multi-provider LLM support** (OpenAI, Anthropic, Ollama) with profile system
- **Confidence scoring in RAG** -- uncommon and valuable differentiator
- **Ed25519 license validation** -- cryptographically sound
- **Helm chart** -- production-grade Kubernetes deployment

### 4.2 Code to Preserve (with refactoring)
- `knowledge-tree-types/` -- Most types directly reusable
- `knowledge-tree-sdk/` -- Plugin framework, crypto, resilience patterns
- `knowledge-tree-core/pkg/storage/` -- PostgreSQL + AGE + pgvector storage layer
- `knowledge-tree-core/pkg/orchestrator/` -- Discovery pipeline and scheduler
- `knowledge-tree-core/pkg/mcp/` -- MCP server implementation
- `knowledge-tree-core/cmd/discover/` -- CLI commands
- `knowledge-tree-core/plugins/` -- 6 plugins (AWS, Azure, GCP, K8s, DNS, GitHub)
- `kt-rag/` -- RAG pipeline, scoring, metadata filtering
- `knowledge-tree-frontend/` -- React SPA (needs component decomposition)
- All 22 standalone plugins -- architecture is sound

### 4.3 Deployment Infrastructure to Preserve
- Docker Compose development setup
- Helm chart (with updates for new architecture)
- CI verification script concept

---

## 5. What Needs Refactoring

### 5.1 Critical Issues

| Issue | Location | Severity | Description |
|-------|----------|----------|-------------|
| Data races | auth (seedUsers), api (Handler fields) | **Critical** | Concurrent map writes without synchronization |
| Weak password hashing | auth (SHA-256 + static salt) | **Critical** | Must use bcrypt/argon2id |
| Hardcoded JWT secret | auth, api, middleware | **High** | Default secret in source code |
| No persistence in microservices | auth, graph, docs | **High** | All state lost on restart |
| Mutable Handler fields | api | **High** | config, licenseTier written without locks |
| No write API on Graph | graph | **High** | Graph service can't be populated via API |
| Stubbed Cypher query | graph | **High** | Returns all resources regardless of query |
| 10,000 resource hard limit | api | **Medium** | Silent truncation at scale |
| Bubble sort | api (coverageHistory) | **Low** | O(n^2) sort |
| Compiled binaries in repo | auth, graph, api | **Low** | Bloats repository |

### 5.2 Architectural Improvements Needed

1. **Eliminate unnecessary microservices:** Auth, Graph, and Docs services add operational complexity without clear benefit. Their functionality can be modules within a unified service.
2. **Persistent storage everywhere:** In-memory backends are useful for development but every service needs the PostgreSQL path to be the default.
3. **Unified error handling:** No consistent error model across services.
4. **Rate limiting and CORS:** Inconsistently applied.
5. **API versioning strategy:** v1 prefix exists but no migration path.
6. **OpenAPI specification:** API docs are hand-maintained TypeScript, not generated from code.

---

## 6. What Should Be Removed

| Component | Reason |
|-----------|--------|
| trafalgar/ | Separate product, not part of Zerelio |
| trafalgar-demo/ | Separate product |
| knowledge-tree-marketing/ | Marketing content, not engineering artifact |
| knowledge-tree-web/ | Marketing site, build separately |
| knowledge-tree-web-apidocs/ | Rebuild from OpenAPI spec |
| knowledge-tree-web-docs/ | Rebuild with proper content system |
| medium/ | Blog drafts |
| generate-deck.js, pitch-deck.html, *.pptx | Business artifacts |
| admin-light.png, graph-grouped.png | Screenshots |
| *.db, *.rvf, compiled binaries | Artifacts that shouldn't be in source |
| instructions.md, instructions_2.md | Agent QA instructions (meta, not product) |
| hopohopo-pitch.md | Unrelated |
| mfm-pitch-deck-generator.skill | Tool artifact |

---

## 7. Missing Capabilities

### 7.1 For Zerelio Knowledge (OSS)

| Capability | Status | Priority |
|------------|--------|----------|
| Persistent storage as default | Partial (PG backend exists) | **P0** |
| Working discovery pipeline end-to-end | Partial (plugins exist, wiring incomplete) | **P0** |
| Graph write API | Missing | **P0** |
| Real Cypher/query language | Stubbed | **P0** |
| Entity resolution | Missing | **P1** |
| Semantic/hybrid search | Backend exists, not exposed well | **P1** |
| Knowledge lifecycle management | Missing | **P1** |
| Webhook/event system | Partial | **P1** |
| Multi-tenancy | Middleware exists but not applied | **P2** |
| OpenAPI spec | Missing | **P2** |
| Proper test coverage | ~30% estimated | **P1** |

### 7.2 For Zerelio Security (Commercial)

| Capability | Status | Priority |
|------------|--------|----------|
| Security intelligence ingestion | Missing | **P0** |
| CVE/CWE integration | Missing | **P0** |
| MITRE ATT&CK mapping | Missing | **P1** |
| Cloud security posture | Assessment engine exists but basic | **P1** |
| Compliance framework | Basic OPA engine exists | **P1** |
| Security knowledge graph | Can build on Knowledge graph | **P1** |
| Threat intelligence | Missing | **P2** |

---

## 8. Technical Debt Inventory

| Debt Item | Location | Impact |
|-----------|----------|--------|
| Duplicate Claims struct | auth handler + middleware | Maintenance burden |
| Local replace directives in go.mod | auth, docs, license | Breaks external builds |
| init() in migrations.go | core storage | Violates project conventions |
| CLAUDE.md references wrong paths | core | Misleads AI assistants |
| Compiled binaries in repo | auth, graph, api | Repository bloat |
| Unused dependencies (tenacity, asyncpg) | kt-rag | Dead code |
| Dockerfile CMD mismatch | kt-rag | Wrong server started in container |
| CodeQL disabled | workflows | Security scanning gap |
| No logout endpoint | auth | Session management gap |
| No refresh token rotation | auth | Security best practice violation |
| Global mutable state in query.py | kt-rag | Thread safety issue |
| 617-line demo interceptor | frontend | Maintenance burden |
| 1530-line Admin.tsx | frontend | Needs decomposition |

---

## 9. Existing License Model

- **Apache 2.0** open source license
- Three-tier commercial licensing: Community (free, 50 resources), Team (500), Enterprise (unlimited)
- Ed25519-signed JWT license keys
- License enforcement in API gateway checking resource counts against tier limits

---

## 10. Migration Strategy Summary

### Preserve and Adapt
- knowledge-tree-types -> zerelio-types (rename, extend)
- knowledge-tree-sdk -> zerelio-sdk (rename, keep plugin architecture)
- knowledge-tree-core/pkg/storage -> zerelio-knowledge/storage (adapt)
- knowledge-tree-core/pkg/orchestrator -> zerelio-knowledge/orchestrator (adapt)
- knowledge-tree-core/pkg/mcp -> zerelio-knowledge/mcp (adapt)
- All 22 plugins -> zerelio-knowledge/plugins (rename, update imports)
- kt-rag -> zerelio-knowledge/rag (integrate, fix storage)
- knowledge-tree-frontend -> zerelio-frontend (rename, decompose)
- Helm chart -> adapt for new architecture

### Rewrite
- Auth service -> proper auth module with argon2id, persistent storage, token rotation
- Graph service -> merge into core with real query language
- API gateway -> unified service with proper concurrency safety
- Docs service -> merge into core with persistent storage

### Remove
- All marketing/pitch/blog content
- Trafalgar (separate product)
- Compiled binaries and database files
- Agent QA instruction files

---

## 11. Key Findings for Architecture Phase

1. **The microservice split adds complexity without proportional benefit.** Auth, Graph, and Docs have 500-2000 lines each and communicate via HTTP. A modular monolith would be simpler and more reliable while preserving the plugin isolation where it matters (discovery).

2. **The plugin architecture is the best-designed component.** The hashicorp/go-plugin + gRPC approach with process isolation is production-grade and should be preserved as-is.

3. **PostgreSQL + AGE + pgvector is the right storage foundation.** The three-subsystem approach (graph/metadata/vector) maps well to the domain. Needs to become the mandatory production backend.

4. **The RAG pipeline is well-conceived** but needs better storage (not JSON files) and tighter integration with the main platform.

5. **The project's own 360-degree audit (in knowledge-tree-knowledge) scored it 5/10** and identified the discovery pipeline as 2/10. This is honest and aligns with my assessment.

6. **128+ "phantom features"** (documented but unimplemented) were previously identified and eliminated. The current codebase still has some (Cypher query stub, many admin endpoints with minimal implementation).

7. **The OSS/commercial boundary is already partially designed** through the license tier system. Zerelio can build on this with stricter module isolation.

8. **Trafalgar integration research exists** but Trafalgar is a separate product. The integration concepts (shared knowledge, MCP connection) are relevant to Zerelio's extensibility design.
