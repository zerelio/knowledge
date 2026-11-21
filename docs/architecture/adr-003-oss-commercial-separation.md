# ADR-003: Open Source / Commercial Separation Model

**Status:** Accepted
**Date:** 2026-11-21
**Decision Makers:** Principal Architect

## Context

Zerelio must support two distinct products:
- **Zerelio Knowledge:** Open-source knowledge infrastructure (Apache-2.0)
- **Zerelio Security:** Commercial security intelligence product

The separation must be enforceable at build time, not just by convention. The OSS product must be genuinely useful standalone — not artificially restricted to drive commercial upgrades.

## Decision

### Dependency Direction (Strictly Enforced)

```
security/ --depends-on--> knowledge/pkg/types
security/ --depends-on--> knowledge/pkg/sdk
security/ --depends-on--> knowledge/ (versioned import)

knowledge/ --NEVER-depends-on--> security/
```

### Enforcement Mechanisms

1. **Go module boundaries:** `knowledge/go.mod` has no reference to `security/`. Compilation fails if this is violated.

2. **CI verification script:** Runs `go list -m all` on the knowledge module and asserts no `github.com/zerelio/security` dependency appears.

3. **Build separation:** `knowledge/` can be built, tested, and distributed independently:
   ```bash
   cd knowledge && go build ./... && go test ./...
   ```

4. **Distribution:** OSS releases are built from `knowledge/` only. The `security/` directory is excluded from OSS tarballs and Docker images.

### Product Boundary Rules

| Capability | Knowledge (OSS) | Security (Commercial) |
|---|---|---|
| Knowledge graph storage | Yes | Extends with security ontology |
| Multi-cloud discovery | Yes (22+ plugins) | Additional security-focused plugins |
| Semantic/hybrid search | Yes | Security-enriched search |
| RAG infrastructure | Yes | Security context RAG |
| LLM documentation | Yes | Security-focused generation |
| MCP server | Yes | Security tools added |
| Plugin SDK | Yes | Security plugin extensions |
| CLI | Yes | Security subcommands |
| RBAC | Yes (standard roles) | Extended security roles |
| Audit logging | Yes (basic) | Enhanced with security events |
| API | Yes (full REST API) | Security API extensions |
| Compliance engine | Basic (OPA) | Full security compliance |
| CVE/CWE intelligence | No | Yes |
| MITRE ATT&CK mapping | No | Yes |
| Threat intelligence | No | Yes |
| Cloud security posture | No | Yes |
| Security knowledge graph | No | Yes |

### Extension Model

Security extends Knowledge through:

1. **Additional plugins:** Security-specific discovery plugins registered via the standard plugin SDK.
2. **API route extensions:** Security registers additional HTTP routes on the unified API server.
3. **Graph extensions:** Security adds security-specific node and edge types to the knowledge graph.
4. **Storage extensions:** Security uses Knowledge's storage interfaces with additional tables/schemas.
5. **Event hooks:** Security subscribes to Knowledge's event bus for security-relevant events.

Knowledge provides extension points but does not know about Security.

## Licensing

### Zerelio Knowledge
- **License:** Apache-2.0
- **Patent grant:** Yes (Apache-2.0 includes patent grant)
- **Commercial use:** Unrestricted
- **Contribution:** CLA required (for patent protection)

### Zerelio Security
- **License:** Commercial (proprietary)
- **Distribution:** Binary or source license
- **Tied to:** Valid commercial subscription

### Why Apache-2.0 (not MIT)

- Apache-2.0 includes an explicit patent grant, protecting users and contributors
- Apache-2.0 is the most common license for enterprise-grade open-source infrastructure
- Compatible with virtually all other open-source licenses
- Preferred by enterprise legal teams over MIT for substantive projects

## Consequences

**Positive:**
- OSS users get a genuinely complete knowledge platform
- Commercial features are clearly delineated
- Enforcement is automated, not dependent on human review
- Third-party plugin authors work with the same public API as Security
- Enterprise adoption is not gated by artificial restrictions

**Negative:**
- Security must work within Knowledge's extension points (cannot modify core internals)
- Features that straddle the boundary require careful API design

## Alternatives Considered

1. **Open core with feature flags:** Rejected because feature flags in a single binary mix OSS and commercial code, making license compliance harder.
2. **BSL (Business Source License):** Rejected because BSL restricts production use, which contradicts the goal of genuine open-source adoption.
3. **AGPL:** Rejected because AGPL's network copyleft would deter enterprise adoption of the OSS product.
4. **Separate repository for Security:** Acceptable alternative but deferred; monorepo is simpler at this stage and the module boundary achieves the same isolation.
