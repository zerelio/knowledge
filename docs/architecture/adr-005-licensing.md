# ADR-005: Licensing Strategy

**Status:** Accepted
**Date:** 2026-11-21
**Decision Makers:** Principal Architect

## Context

Zerelio needs a licensing strategy that:
1. Encourages adoption of the open-source Knowledge product
2. Protects the commercial Security product
3. Avoids dependency license conflicts
4. Provides patent protection for contributors and users

## Decision

### Zerelio Knowledge: Apache License 2.0

**Chosen over MIT because:**
- Apache-2.0 includes an explicit patent grant (Section 3), protecting users from patent claims by contributors
- Apache-2.0 includes a patent termination clause — contributors who sue users over patents lose their license
- Apache-2.0 is the standard for enterprise infrastructure projects (Kubernetes, Terraform, Apache Kafka, etc.)
- Apache-2.0 is compatible with GPLv3 (important for users integrating with GPL tools)
- Enterprise legal teams prefer Apache-2.0 for its clarity on patents

**Not chosen: BSL, SSPL, AGPL**
- BSL (MariaDB, CockroachDB) restricts production use — contradicts genuine open-source goal
- SSPL (MongoDB) is not OSI-approved and deters cloud providers from offering hosted versions (which is a feature, not a bug, but also deters enterprise adoption)
- AGPL (Neo4j Community) requires source disclosure for network services — deters enterprise internal use

### Zerelio Security: Proprietary Commercial License

- Standard commercial software license
- Per-seat or per-node pricing (TBD)
- Source-available for enterprise customers (not open-source)
- No redistribution rights

### Contributor License Agreement (CLA)

A CLA is required for Zerelio Knowledge contributions to:
- Ensure Zerelio maintains the right to license the code
- Enable future re-licensing if necessary (e.g., to a later Apache version)
- Provide explicit patent grants from contributors
- Standard Apache CLA is recommended (well-understood, minimal friction)

## Dependency License Audit

### Critical Dependencies and Their Licenses

| Dependency | License | Compatible with Apache-2.0? | Notes |
|------------|---------|----------------------------|-------|
| PostgreSQL | PostgreSQL License (permissive) | Yes | |
| Apache AGE | Apache-2.0 | Yes | |
| pgvector | PostgreSQL License | Yes | |
| go-chi/chi | MIT | Yes | |
| hashicorp/go-plugin | MPL-2.0 | Yes* | MPL-2.0 is file-level copyleft; we use it as a library, so our code stays Apache-2.0 |
| golang-jwt/jwt | MIT | Yes | |
| spf13/cobra | Apache-2.0 | Yes | |
| spf13/viper | MIT | Yes | |
| OpenTelemetry | Apache-2.0 | Yes | |
| jackc/pgx | MIT | Yes | |
| robfig/cron | MIT | Yes | |
| bbolt | MIT | Yes | |
| AWS SDK v2 | Apache-2.0 | Yes | |
| Azure SDK | MIT | Yes | |
| GCP SDK | Apache-2.0 | Yes | |
| k8s client-go | Apache-2.0 | Yes | |
| crewjam/saml | Apache-2.0 | Yes | |
| coreos/go-oidc | Apache-2.0 | Yes | |
| LightRAG (Python) | MIT | Yes | |
| FastAPI | MIT | Yes | |
| sentence-transformers | Apache-2.0 | Yes | |

*hashicorp/go-plugin is MPL-2.0. Mozilla Public License 2.0 is a weak copyleft license that applies file-by-file. Since we import it as a library and don't modify its source files, our Apache-2.0 code is not affected. If we ever need to modify go-plugin source files, those specific files would remain MPL-2.0.

### No GPL Dependencies

The project has **no GPL-licensed dependencies**. This is important because while Apache-2.0 is compatible with GPLv3, mixing GPL code into an Apache-2.0 project would require the entire project to be distributed under GPLv3, losing the permissive license.

**Risk:** Neo4j Community Edition is GPL. This is one of the reasons we chose PostgreSQL + AGE instead of Neo4j — using Neo4j as an embedded library would require GPL compliance for the entire project.

## Consequences

**Positive:**
- Enterprise adoption is frictionless (Apache-2.0 is universally approved)
- Patent protection for all parties
- Clear separation between OSS and commercial code
- No dependency license conflicts
- Contributors have clear expectations via CLA

**Negative:**
- Apache-2.0 allows competitors to use the code without contributing back (by design)
- CLA adds friction to first-time contributors (mitigated: automated CLA bot)
- No copyleft protection means cloud providers could offer Zerelio Knowledge as a service

**Trade-off accepted:** We accept that cloud providers could offer hosted Zerelio Knowledge. The commercial value of Zerelio Security and enterprise support contracts provides the business model, not code restrictions. A permissive license maximizes adoption, which is the priority for an early-stage project.
