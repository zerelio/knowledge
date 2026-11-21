# Migration Matrix: Knowledge Tree -> Zerelio

**Date:** 2026-11-21

## Component Migration Decisions

| Existing Component | Current State | Target Location | Action | Rationale |
|---|---|---|---|---|
| **Core Modules** | | | | |
| knowledge-tree-types | Excellent, zero deps, ~100 resource types | zerelio/knowledge/types | **Adapt** | Rename package paths, extend with knowledge-domain types. Foundation of the type system. |
| knowledge-tree-sdk | Good, gRPC plugin framework + crypto + resilience | zerelio/knowledge/sdk | **Adapt** | Rename, keep plugin architecture. Best-designed component in the project. |
| knowledge-tree-core/pkg/storage | Good, PG + AGE + pgvector, 12 migrations | zerelio/knowledge/storage | **Adapt** | Make PostgreSQL the primary backend. Clean up Backend interface. Remove memory backend as production option (keep for testing). |
| knowledge-tree-core/pkg/orchestrator | Good, pipeline + scheduler + credential broker | zerelio/knowledge/orchestrator | **Adapt** | Clean up, improve error handling, wire to real plugin execution. |
| knowledge-tree-core/pkg/mcp | Good, JSON-RPC 2.0, 8 tools, 4 resources | zerelio/knowledge/mcp | **Adapt** | Extend with additional tools for knowledge operations. |
| knowledge-tree-core/cmd/discover | Good, Cobra CLI | zerelio/knowledge/cmd/zerelio | **Adapt** | Rename to `zerelio` CLI, add subcommands. |
| knowledge-tree-core/agent | Good, mTLS + BoltDB buffering | zerelio/knowledge/agent | **Adapt** | Keep agent architecture, update communication protocol. |
| knowledge-tree-core/pkg/docs | Fair, markdown/mermaid generation | zerelio/knowledge/docs | **Refactor** | Merge docs microservice functionality, add persistent storage. |
| knowledge-tree-core/pkg/runbook | Fair, LLM-based generation | zerelio/knowledge/runbook | **Refactor** | Improve prompts, add template system. |
| knowledge-tree-core/pkg/knowledge | Fair, knowledge store/compiler | zerelio/knowledge/knowledge | **Refactor** | Extend for proper knowledge lifecycle. |
| knowledge-tree-core/pkg/change | Good, detection + notification | zerelio/knowledge/change | **Adapt** | Keep change detection, improve webhook delivery. |
| knowledge-tree-core/pkg/events | Good, in-process event bus | zerelio/knowledge/events | **Keep** | Simple, effective. |
| knowledge-tree-core/pkg/compliance | Basic, OPA engine | zerelio/knowledge/compliance | **Refactor** | Extend for Zerelio Security. |
| knowledge-tree-core/pkg/assessment | Basic | zerelio/knowledge/assessment | **Refactor** | Extend for cloud assessment. |
| knowledge-tree-core/pkg/credentials | Good, multi-provider | zerelio/knowledge/credentials | **Keep** | AWS, Azure, GCP, K8s, Vault credential providers. |
| knowledge-tree-core/pkg/telemetry | Good, OpenTelemetry | zerelio/knowledge/telemetry | **Keep** | Standard observability. |
| knowledge-tree-core/pkg/errors | Good | zerelio/knowledge/errors | **Keep** | Custom error types. |
| knowledge-tree-core/pkg/cli | Good, progress/formatting helpers | zerelio/knowledge/cli | **Keep** | CLI utilities. |
| knowledge-tree-core/pkg/seed | Fair, sample data | zerelio/knowledge/seed | **Refactor** | Improve demo data quality. |
| **Microservices** | | | | |
| knowledge-tree-api | Fair, 100+ endpoints, races, no persistence for some state | zerelio/knowledge/api | **Rewrite** | Merge into unified service. Fix concurrency. Make all state persistent. |
| knowledge-tree-auth | Poor, data races, SHA-256 passwords, in-memory | zerelio/knowledge/auth | **Rewrite** | Proper auth module: argon2id, persistent sessions, token rotation, RBAC enforcement. |
| knowledge-tree-docs (service) | Fair, LLM integration works, in-memory store | Merge into zerelio/knowledge/api | **Merge** | Eliminate separate microservice. Keep LLM client and retriever logic. |
| knowledge-tree-graph (service) | Poor, no write API, stubbed Cypher | Merge into zerelio/knowledge/api | **Merge** | Graph operations become part of the storage layer, not a separate service. |
| knowledge-tree-license | Good, Ed25519 JWT | zerelio/knowledge/license | **Adapt** | Keep validation, extend tier model for Zerelio products. |
| **Plugins** | | | | |
| plugins/aws | Good, 770 LOC, multi-region | zerelio/knowledge/plugins/aws | **Adapt** | Rename imports, verify discovery works end-to-end. |
| plugins/azure | Good | zerelio/knowledge/plugins/azure | **Adapt** | Same. |
| plugins/gcp | Good | zerelio/knowledge/plugins/gcp | **Adapt** | Same. |
| plugins/k8s | Good | zerelio/knowledge/plugins/k8s | **Adapt** | Same. |
| plugins/dns | Good, sophisticated AXFR | zerelio/knowledge/plugins/dns | **Adapt** | Same. |
| plugins/github | Fair, compilation bug | zerelio/knowledge/plugins/github | **Fix + Adapt** | Fix duplicate var declaration, rename imports. |
| plugins/codegraph | Good, SQLite reader | zerelio/knowledge/plugins/codegraph | **Adapt** | Interesting code intelligence plugin. |
| plugins/grafana | Good | zerelio/knowledge/plugins/grafana | **Adapt** | Same. |
| plugins/jira | Fair | zerelio/knowledge/plugins/jira | **Adapt** | Same. |
| plugins/datadog | Fair | zerelio/knowledge/plugins/datadog | **Adapt** | Same. |
| plugins/mongodb | Fair, unusual HTTP API | zerelio/knowledge/plugins/mongodb | **Review** | HTTP API pattern needs investigation. |
| plugins/mysql | Fair, unusual HTTP API | zerelio/knowledge/plugins/mysql | **Review** | Same. |
| plugins/postgresql | Fair, unusual HTTP API | zerelio/knowledge/plugins/postgresql | **Review** | Same. |
| plugins/vsphere | Good, Go generics | zerelio/knowledge/plugins/vsphere | **Adapt** | Same. |
| plugins/cloudflare | Good | zerelio/knowledge/plugins/cloudflare | **Adapt** | Same. |
| plugins/pagerduty | Good | zerelio/knowledge/plugins/pagerduty | **Adapt** | Same. |
| plugins/confluence | Good | zerelio/knowledge/plugins/confluence | **Adapt** | Same. |
| plugins/slack | Good | zerelio/knowledge/plugins/slack | **Adapt** | Same. |
| plugins/servicenow | Good | zerelio/knowledge/plugins/servicenow | **Adapt** | Same. |
| plugins/newrelic | Good | zerelio/knowledge/plugins/newrelic | **Adapt** | Same. |
| plugins/prometheus | Good | zerelio/knowledge/plugins/prometheus | **Adapt** | Same. |
| plugins/elasticsearch | Good | zerelio/knowledge/plugins/elasticsearch | **Adapt** | Same. |
| **Frontend** | | | | |
| knowledge-tree-frontend | Good, comprehensive React SPA | zerelio/frontend | **Refactor** | Rename, decompose large components, add proper error boundaries, wire TanStack Query. |
| **RAG** | | | | |
| kt-rag | Good, 43 tests, clean Python | zerelio/knowledge/rag | **Adapt** | Fix JSON storage (use PG/pgvector), fix Dockerfile CMD, wire prompt templates. |
| **Infrastructure** | | | | |
| deploy/docker/ | Good | zerelio/deploy/docker | **Adapt** | Update for new service architecture. |
| deploy/aws/ | Good | zerelio/deploy/aws | **Adapt** | Same. |
| deploy/helm-chart/ | Good | zerelio/deploy/helm | **Adapt** | Update for Zerelio branding and architecture. |
| knowledge-tree-workflows | Good | zerelio/.github/workflows | **Adapt** | Update for new module structure. |
| scripts/ci-verify.sh | Good | zerelio/scripts/ci-verify.sh | **Adapt** | Update module paths and boundary checks. |
| **Documentation** | | | | |
| knowledge-tree-knowledge/ | Valuable knowledge base | zerelio/docs/ | **Preserve selectively** | ADRs, audit findings, and research are valuable context. |
| **Remove** | | | | |
| trafalgar/ | Separate product | N/A | **Remove** | Not part of Zerelio. |
| trafalgar-demo/ | Separate product | N/A | **Remove** | Not part of Zerelio. |
| knowledge-tree-marketing/ | Marketing content | N/A | **Remove** | Rebuild for Zerelio brand. |
| knowledge-tree-web/ | Marketing site | N/A | **Remove** | Rebuild for Zerelio. |
| knowledge-tree-web-apidocs/ | Hand-maintained API docs | N/A | **Remove** | Generate from OpenAPI spec. |
| knowledge-tree-web-docs/ | Product docs (many stubs) | N/A | **Remove** | Rebuild with proper content system. |
| knowledge-tree-billing/ | Stub (README only) | zerelio/security/billing (future) | **Defer** | No code to migrate. |
| medium/ | Blog drafts | N/A | **Remove** | Not engineering artifact. |
| *.pptx, *.png, *.skill, *.html | Business artifacts | N/A | **Remove** | Not engineering artifacts. |
| compiled binaries, .db files | Build artifacts | N/A | **Remove** | Should never be in source control. |

## Summary Statistics

| Action | Count | Description |
|--------|-------|-------------|
| **Keep** | 7 | Directly reusable with minimal changes |
| **Adapt** | 32 | Rename imports, update paths, minor improvements |
| **Refactor** | 7 | Significant restructuring needed |
| **Rewrite** | 2 | Auth module and API gateway need ground-up rewrite |
| **Merge** | 2 | Docs and Graph services merge into unified API |
| **Fix + Adapt** | 1 | GitHub plugin has a compilation bug |
| **Review** | 3 | MongoDB/MySQL/PostgreSQL plugins need API pattern investigation |
| **Remove** | 10+ | Marketing, Trafalgar, business artifacts |
| **Defer** | 1 | Billing (no code exists) |
