# ADR-001: Modular Monolith over Microservices

**Status:** Accepted
**Date:** 2026-11-21
**Decision Makers:** Principal Architect

## Context

Knowledge Tree uses a microservices architecture with 5 separate HTTP services (API gateway :8080, Auth :8083, Docs :8081, Graph :8082, License :8084) plus a Python RAG service (:8085). The services communicate via HTTP proxying from the API gateway.

In practice, the Auth, Docs, and Graph services are small (500-2000 lines each), all run on the same machine in development, and share the same PostgreSQL database in production. The API gateway already imports `knowledge-tree-core` as a Go library. The microservice separation creates operational complexity (5 processes to start, inter-service authentication, network latency, deployment coordination) without the benefits microservices are designed to provide (independent scaling, independent deployment, team isolation).

## Decision

Zerelio Knowledge will use a **modular monolith** architecture with strict internal module boundaries.

A single Go binary (`zerelio-server`) will contain:
- HTTP API layer (unified router)
- Auth module (authentication, authorization, RBAC)
- Knowledge module (storage, graph, search, retrieval)
- Discovery module (orchestrator, scheduler, plugin management)
- Documentation module (LLM generation, runbooks, context packing)
- License module (validation, tier enforcement)
- MCP module (Model Context Protocol server)
- Observability module (telemetry, audit, metrics)

The RAG service remains a separate Python process because it uses Python-specific libraries (LightRAG, sentence-transformers) that cannot run in Go.

Plugin binaries remain separate processes (hashicorp/go-plugin over gRPC) because process isolation for third-party code is a genuine safety requirement.

## Consequences

**Positive:**
- Single binary deployment simplifies operations
- No inter-service HTTP latency for internal calls
- Shared database connections (connection pool efficiency)
- Simpler development experience (one `go run`)
- Easier transaction management across modules
- Easier debugging and tracing

**Negative:**
- All modules scale together (mitigated: knowledge platforms are not typically CPU-bound on a single axis)
- Requires strict internal module boundaries to prevent coupling (mitigated: Go package visibility, CI enforcement)
- Modules cannot use different languages (mitigated: only the RAG service needs Python)

**Trade-offs accepted:**
- We lose independent per-module scaling. Acceptable because the platform is I/O bound (database, LLM API calls) not compute-bound, and horizontal scaling of the monolith handles this.
- We gain operational simplicity. Critical for an open-source project where self-hosting ease is a competitive advantage.

## Alternatives Considered

1. **Keep microservices:** Rejected because the current services are too small to justify separate processes and the HTTP proxying adds latency and complexity.
2. **Service mesh (Envoy/Istio):** Rejected as massive overengineering for this scale.
3. **Modular monolith + extract later:** This is our approach. If a module genuinely needs independent scaling in the future, the module boundaries make extraction possible.

## Module Boundary Enforcement

- Each module lives in its own Go package under `internal/`
- Modules communicate through defined Go interfaces, not HTTP
- CI verifies that no module imports another module's internal packages
- The API layer is the only package that imports all modules
- Public types shared across modules live in `pkg/types/`
