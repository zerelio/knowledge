# ADR-002: Repository Structure

**Status:** Accepted
**Date:** 2026-11-21
**Decision Makers:** Principal Architect

## Context

Zerelio consists of two products:
- **Zerelio Knowledge** (open-source): General-purpose knowledge infrastructure platform
- **Zerelio Security** (commercial): Security intelligence product built on Knowledge

The repository must accommodate both products with strict separation, enable independent OSS distribution, and support a plugin ecosystem with 22+ existing plugins.

## Decision

Use a **monorepo with Go workspace** and strict product boundaries.

```
zerelio/
  go.work                          # Go workspace definition
  LICENSE                           # Apache-2.0 (applies to knowledge/)
  AGENTS.md                         # Agent entry point
  README.md

  knowledge/                        # Zerelio Knowledge (Apache-2.0)
    go.mod                          # github.com/zerelio/knowledge
    cmd/
      zerelio/                      # CLI binary (Cobra)
      zerelio-server/               # Server binary
      zerelio-agent/                # Remote agent binary
    pkg/
      types/                        # Shared types (zero external deps)
      sdk/                          # Plugin SDK + crypto + resilience
    internal/
      api/                          # HTTP API layer (chi router)
      auth/                         # Authentication + authorization
      storage/                      # PostgreSQL + AGE + pgvector
      graph/                        # Graph operations + query language
      search/                       # Hybrid search (vector + keyword + graph)
      discovery/                    # Orchestrator + scheduler + pipeline
      docs/                         # LLM documentation generation
      runbook/                      # Runbook generation
      knowledge/                    # Knowledge lifecycle management
      mcp/                          # Model Context Protocol server
      license/                      # License validation
      compliance/                   # Compliance engine
      events/                       # Event bus
      change/                       # Change detection + notification
      telemetry/                    # OpenTelemetry
      credentials/                  # Multi-cloud credential providers
    plugins/                        # Built-in discovery plugins
      aws/
      azure/
      gcp/
      k8s/
      dns/
      github/
      ... (22 plugins, each with go.mod)
    rag/                            # Python RAG service
      pyproject.toml
      ...
    deploy/
      docker/
      helm/
      aws/
    tests/
      integration/
      e2e/

  security/                         # Zerelio Security (Commercial License)
    go.mod                          # github.com/zerelio/security
    LICENSE                         # Commercial license
    internal/
      intel/                        # Security intelligence ingestion
      vuln/                         # CVE/CWE vulnerability management
      threat/                       # Threat intelligence
      posture/                      # Cloud security posture
      compliance/                   # Security compliance frameworks
      graph/                        # Security knowledge graph extensions
      api/                          # Security API extensions
    plugins/
      ... (security-specific plugins)

  frontend/                         # React SPA (shared between products)
    package.json
    src/
    ...

  docs/
    phase-zero/
    research/
    architecture/
    project-memory/
    execution-log/
```

## Key Design Decisions

### Separate go.mod per product
- `knowledge/go.mod` and `security/go.mod` are independent Go modules
- `security` imports `knowledge` as a versioned dependency
- `knowledge` NEVER imports `security`
- CI enforces this dependency direction

### Plugins as separate Go modules
- Each plugin has its own `go.mod` in `knowledge/plugins/<name>/`
- Plugins depend only on `knowledge/pkg/sdk` (and transitively `knowledge/pkg/types`)
- Plugins are built as separate binaries
- Go workspace (`go.work`) links everything for local development

### Frontend as shared project
- Single React SPA serves both Knowledge and Security
- Feature flags or routing determine which product surfaces appear
- Licensed features are gated by the backend, not the frontend

### pkg/ vs internal/
- `pkg/types/` and `pkg/sdk/` are public API — stable, versioned, imported by plugins
- `internal/` packages are private — can change freely without breaking plugins

## Consequences

**Positive:**
- Clear product ownership boundary at the directory level
- OSS distribution can exclude `security/` directory entirely
- Plugins only depend on public API (`pkg/`), insulated from internal changes
- Go workspace enables local development across all modules
- Single repository simplifies cross-cutting changes

**Negative:**
- Monorepo requires tooling for selective builds and CI
- Both products share git history (mitigated: OSS distribution is a build artifact, not a git fork)

## Alternatives Considered

1. **Separate repositories:** Rejected because cross-cutting changes (type updates, SDK changes) would require coordinated PRs across repos. The overhead is not justified at this stage.
2. **Single go.mod:** Rejected because it would make the OSS module carry commercial dependencies.
3. **Git subtree/submodule:** Rejected due to operational complexity and poor developer experience.
