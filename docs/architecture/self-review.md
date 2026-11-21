# Phase Three: Architectural Self-Review

**Date:** 2026-11-21

Three independent reviewer perspectives evaluate the Zerelio architecture before implementation.

---

## Reviewer A: Distinguished Systems Architect

### Overengineering Assessment

**Verdict: Architecture is appropriately scoped.** The modular monolith decision (ADR-001) actively avoids the most common overengineering mistake. The module list is reasonable — each module maps to a distinct domain concern. No unnecessary abstraction layers.

**One concern:** The architecture documents describe capabilities (entity resolution, knowledge lifecycle management, provenance tracking) that should be deferred until core functionality works end-to-end. Risk of scope creep if implementation tries to build all described modules simultaneously.

**Recommendation:** Implement in strict priority order. Core path first: storage -> discovery -> graph -> search -> API -> CLI. Advanced features (entity resolution, provenance, compliance) are Phase 2.

### Coupling Analysis

**The pkg/types boundary is correct.** Public types in `pkg/types/` and `pkg/sdk/` form the stable API surface. Internal modules communicate through Go interfaces, not HTTP, which eliminates the distributed coupling problem of the original microservices.

**Risk:** The unified API layer (`internal/api/`) will become a coupling magnet if it directly calls into module internals. 

**Recommendation:** Define a `Service` interface per module that the API layer calls. Example: `discovery.Service`, `graph.Service`, `docs.Service`. This keeps the API layer thin and modules independently testable.

### Scalability

**Adequate for the expected scale.** A knowledge platform serving infrastructure data for organizations with 10K-100K resources is I/O-bound (database queries, cloud API calls, LLM requests), not CPU-bound. The modular monolith scales horizontally for this workload.

**Bottleneck identified:** The RAG service as a separate Python process introduces a network hop for every AI query. If AI-powered retrieval becomes the primary access pattern, this becomes the latency bottleneck.

**Recommendation:** Accept the Python sidecar pattern for now. If latency becomes an issue, evaluate Go-native embedding inference (via ONNX runtime) to eliminate the sidecar for the retrieval path.

### Data Architecture

**PostgreSQL + AGE + pgvector is defensible.** The single-database approach trades peak performance for operational simplicity — the right trade-off for an open-source self-hosted product.

**Concern:** Apache AGE's Cypher implementation is incomplete compared to Neo4j's. Specifically: variable-length path patterns, aggregation within path expressions, and some APOC procedures are missing.

**Recommendation:** Document the supported Cypher subset explicitly. Don't promise full Cypher compatibility. For complex graph algorithms (PageRank, community detection), implement them as Go functions that read from PostgreSQL rather than trying to do them in Cypher.

---

## Reviewer B: Principal Open-Source Architect

### OSS Product Completeness

**Verdict: The OSS product is genuinely useful.** Zerelio Knowledge includes all critical capabilities: multi-cloud discovery, knowledge graph, search, AI documentation, MCP integration, CLI, and deployment tooling. Nothing essential is artificially gated behind the commercial product.

**Concern:** The compliance engine is listed as basic (OPA) in Knowledge and full in Security. Compliance is a core knowledge management capability, not a security-only feature. If the OSS compliance engine is too limited, it undermines the product's value proposition.

**Recommendation:** Give Knowledge a solid compliance engine (custom policies, evaluation, basic reporting). Security adds security-specific frameworks (CIS benchmarks, NIST, SOC 2) and advanced remediation workflows.

### Contributor Experience

**Good foundations.** Apache-2.0 license is contributor-friendly. Go is accessible to infrastructure engineers. The plugin SDK provides a clear extension point.

**Concerns:**
1. CLA adds friction. Consider DCO (Developer Certificate of Origin) as a lighter alternative for initial adoption.
2. No contribution guide or development environment documentation exists yet.
3. The monorepo Go workspace setup requires Go 1.26+ which is very recent.

**Recommendations:**
1. Start with DCO (signed-off-by lines in commits). Move to CLA only if patent concerns arise.
2. Create CONTRIBUTING.md with setup instructions before announcing the project.
3. Provide a `make dev-setup` target that installs prerequisites and validates the environment.

### Adoption Barriers

**Identified barriers:**
1. **PostgreSQL requirement:** Users must install PostgreSQL with AGE and pgvector extensions. This is non-trivial.
2. **LLM API key:** Documentation and RAG features require an LLM provider.
3. **Plugin binaries:** Users must build or download 22+ plugin binaries.

**Recommendations:**
1. Provide a Docker Compose one-liner that starts everything including PostgreSQL with extensions pre-installed.
2. Support Ollama as a local LLM option with a clear setup path.
3. Bundle the most common plugins (AWS, Azure, GCP, K8s) into the main binary as built-in plugins. Keep the external plugin system for community/third-party plugins.

### Build and Deployment Experience

**Concern:** The architecture describes three separate binaries (zerelio-server, zerelio agent, zerelio CLI) plus a Python RAG service plus 22+ plugin binaries. That's 25+ binaries to build.

**Recommendation:** Reduce the binary count:
- Single `zerelio` binary with subcommands: `zerelio serve`, `zerelio discover`, `zerelio agent`
- Bundle top-6 plugins (AWS, Azure, GCP, K8s, GitHub, DNS) as in-process plugins (no gRPC overhead)
- Keep the external plugin system for the remaining 16+ plugins

### Public API Stability

**Good.** The `pkg/types/` and `pkg/sdk/` separation creates a clear stable/unstable boundary. Plugins depend only on stable packages.

**Recommendation:** Version the SDK with semver. Breaking changes to `pkg/sdk/` must increment the major version. Add a `//go:build` constraint or module major version bump so old plugins fail to compile rather than silently breaking.

---

## Reviewer C: Principal Security Architect

### Trust Boundaries

**The architecture correctly identifies three trust boundaries:**
1. External network -> TLS termination -> API layer (untrusted input)
2. API layer -> Plugin processes (credential-bearing, sandboxed)
3. API layer -> LLM providers (sensitive data exfiltration risk)

**Concern:** The plugin trust boundary deserves more attention. Plugins receive cloud credentials and make external API calls. A malicious or compromised plugin could exfiltrate credentials.

**Recommendations:**
1. Plugin credential isolation: plugins receive only the credentials they need for their specific provider, never the full credential store.
2. Network policy: in Kubernetes deployments, restrict plugin pod egress to only the cloud provider API endpoints they need.
3. Plugin signing: require cryptographic signature verification before loading plugins (future phase).
4. Audit plugin API calls: log which plugins access which credentials.

### Authentication and Authorization

**ADR-006 addresses the critical Knowledge Tree auth issues correctly.** Ed25519 signing, argon2id hashing, and refresh token rotation are appropriate choices.

**Additional recommendations:**
1. Implement token revocation via a database-backed token blocklist, not just expiry-based invalidation.
2. Add rate limiting on the login endpoint (5 attempts per minute per IP, with exponential backoff).
3. SCIM token rotation: SCIM bearer tokens should have configurable expiry and support rotation.
4. Consider PASETO tokens as an alternative to JWT — PASETO eliminates algorithm confusion attacks entirely.

### Tenant Isolation

**The architecture mentions PostgreSQL RLS for multi-tenancy but doesn't detail the implementation.**

**Recommendations:**
1. Every table must have a `tenant_id` column with a NOT NULL constraint.
2. RLS policies must be applied to ALL tables, verified by a CI check.
3. The application must set `SET app.current_tenant = ?` on every database connection from the pool.
4. Integration tests must verify cross-tenant data isolation explicitly.
5. Graph queries via Apache AGE must also be tenant-scoped — AGE doesn't inherit RLS automatically.

### Secrets Management

**Concerns:**
1. The encryption key (`KT_ENCRYPTION_KEY`) is loaded from an environment variable. In Kubernetes, this should come from a Secret (or external secrets operator), not ConfigMap.
2. LLM API keys are proxied to external providers. If the server is compromised, all configured LLM API keys are accessible.
3. The default JWT secret in Knowledge Tree (`change-me-in-production-use-32-chars!`) must not be carried forward.

**Recommendations:**
1. Require explicit key configuration — no default secrets. Server refuses to start without `ZERELIO_JWT_PRIVATE_KEY` and `ZERELIO_ENCRYPTION_KEY`.
2. Support external secret stores (Vault, AWS Secrets Manager) for all sensitive configuration.
3. LLM API keys should be stored encrypted at rest and decrypted only for the duration of the API call.
4. Add a security bootstrap command (`zerelio init-keys`) that generates required cryptographic keys.

### Supply Chain Security

**Recommendations:**
1. Pin all Go module dependencies by checksum (Go's default via go.sum).
2. Run `govulncheck` in CI on every PR.
3. Run Trivy filesystem scan for known vulnerabilities.
4. Sign release binaries with Sigstore/cosign.
5. Publish SBOMs (Software Bill of Materials) with each release.
6. Use a minimal base image (distroless or Alpine) for Docker images.

### Data Exposure

**Concern:** The MCP server exposes infrastructure knowledge to AI tools. If the MCP server is accessible to untrusted AI agents, it becomes an information disclosure vector.

**Recommendations:**
1. MCP server should require authentication (API token or mTLS).
2. MCP tool calls should be subject to RBAC — the connecting agent inherits the permissions of the user who authorized it.
3. Add audit logging for all MCP tool invocations.

---

## Summary of Issues Found and Resolutions

| Issue | Reviewer | Severity | Resolution |
|-------|----------|----------|------------|
| Scope creep risk with advanced features | A | Medium | Implement in strict priority order; core path first |
| API layer coupling magnet | A | Medium | Define Service interface per module |
| AGE Cypher subset incomplete | A | Low | Document supported subset explicitly |
| Compliance engine too limited in OSS | B | Medium | Give Knowledge solid compliance; Security adds frameworks |
| CLA friction | B | Low | Start with DCO, move to CLA later |
| PostgreSQL setup complexity | B | Medium | Docker Compose one-liner with extensions pre-installed |
| Too many binaries | B | Medium | Single binary with subcommands; bundle top-6 plugins |
| Plugin credential exfiltration | C | High | Credential isolation, network policy, audit logging |
| No token revocation mechanism | C | Medium | Database-backed token blocklist |
| Tenant isolation incomplete | C | High | RLS on all tables, CI verification, AGE tenant scoping |
| Default secrets must not exist | C | High | Require explicit key config; refuse to start without keys |
| MCP server information disclosure | C | Medium | Require auth, enforce RBAC, audit logging |

### Changes Applied to Architecture

Based on this review, the following changes are incorporated:

1. **Implementation priority order defined** (ADR-001 updated conceptually): storage -> discovery -> graph -> API -> CLI -> search -> docs -> advanced features
2. **Service interface per module** added to system architecture
3. **Compliance engine scope** clarified: Knowledge gets custom policies + evaluation; Security adds frameworks
4. **Single binary design** adopted: `zerelio serve|discover|agent` subcommands
5. **Top-6 plugins bundled** as in-process plugins; external plugin system retained for the rest
6. **Security hardening**: no default secrets, mandatory key configuration, plugin credential isolation
7. **Tenant scoping for AGE** added to data architecture requirements
8. **MCP authentication** added as requirement
