# Zerelio System Architecture

**Date:** 2026-11-21
**Version:** 1.0
**Status:** Draft

---

## 1. Product Architecture

```
                    Zerelio Platform
                         |
           +-------------+-------------+
           |                           |
    Zerelio Knowledge              Zerelio Security
    (Apache-2.0 OSS)              (Commercial)
           |                           |
    +------+------+              +-----+-----+
    | Core Engine |              | Security  |
    | - Storage   |<----uses----| Intel     |
    | - Graph     |              | - CVE/CWE |
    | - Search    |              | - MITRE   |
    | - Discovery |              | - Threat  |
    | - Docs/RAG  |              | - Posture |
    | - MCP       |              | - Comply  |
    | - API       |              +-----------+
    | - CLI       |
    +------+------+
           |
    +------+------+
    | Plugin SDK  |
    | (Public API)|
    +------+------+
           |
    +------+------+
    | 22+ Plugins |
    | (Separate   |
    |  processes) |
    +-------------+
```

## 2. Component Architecture

### 2.1 Zerelio Knowledge — Modular Monolith

```
zerelio-server binary
├── API Layer (chi router, middleware, rate limiting, CORS)
│   ├── Auth Middleware (JWT validation, RBAC enforcement)
│   ├── Public Routes (/health, /login, /register, /sso/*)
│   ├── Authenticated Routes (/api/v1/*)
│   └── Admin Routes (/api/v1/admin/*)
│
├── Auth Module
│   ├── Password Authentication (argon2id)
│   ├── JWT Token Management (Ed25519 signing, token rotation)
│   ├── SSO (OIDC + SAML 2.0)
│   ├── SCIM v2 Provisioning
│   ├── RBAC Engine (roles, permissions, enforcement)
│   └── API Token Management
│
├── Storage Module
│   ├── Graph Store (PostgreSQL + Apache AGE)
│   │   ├── Cypher Query Engine
│   │   ├── Node/Edge CRUD
│   │   ├── Path Finding (BFS/Dijkstra)
│   │   └── Topology Analysis
│   ├── Metadata Store (PostgreSQL)
│   │   ├── Scopes, Runs, Config
│   │   ├── Users, Audit Entries
│   │   └── Schema Migrations (checksummed)
│   ├── Vector Store (pgvector)
│   │   ├── Embedding Storage
│   │   ├── Cosine Similarity Search
│   │   └── IVFFlat Indexing
│   └── Backend Interface (for testing: memory backend)
│
├── Discovery Module
│   ├── Orchestrator (pipeline management)
│   ├── Plugin Manager (hashicorp/go-plugin, gRPC)
│   ├── Scheduler (cron-based)
│   ├── Credential Broker (Vault, AWS SM, env, file)
│   ├── Chain Discovery (cross-cloud recursive)
│   └── IaC Importers (Terraform, Pulumi)
│
├── Knowledge Module
│   ├── Knowledge Lifecycle Manager
│   ├── Entity Resolution
│   ├── Change Detection + Notification
│   ├── Provenance Tracking
│   └── Knowledge Compiler
│
├── Search Module
│   ├── Hybrid Search (BM25 + vector + graph)
│   ├── Faceted Search
│   └── Re-ranking
│
├── Documentation Module
│   ├── LLM Client (OpenAI, Anthropic, Ollama)
│   ├── Document Generator
│   ├── Runbook Generator (3-step pipeline)
│   ├── Context Packer (token budget management)
│   └── Template Engine
│
├── MCP Module
│   ├── JSON-RPC 2.0 Protocol
│   ├── Tools (8+ tools)
│   ├── Resources (4+ categories)
│   └── Transports (stdio, HTTP)
│
├── License Module
│   ├── Ed25519 JWT Validation
│   ├── Tier Enforcement
│   └── Feature Gating
│
├── Compliance Module
│   ├── OPA Policy Engine
│   ├── Policy CRUD
│   └── Evaluation + Reporting
│
├── Events Module
│   ├── In-process Event Bus
│   └── Webhook Delivery (HMAC-signed)
│
└── Observability Module
    ├── OpenTelemetry Integration
    ├── Audit Logging (hash-chained)
    ├── Evidence Collection
    └── Metrics Export
```

### 2.2 External Processes

```
zerelio-agent binary
├── mTLS Communication
├── BoltDB Offline Buffering
├── osquery-style System Queries
└── Heartbeat + Result Submission

zerelio-rag service (Python)
├── FastAPI Server
├── LightRAG Engine
├── Multi-LLM Profile System
├── Confidence Scoring Pipeline
├── Metadata Filtering
└── Incremental Sync

Plugin Binaries (22+)
├── Each: separate OS process
├── gRPC communication (hashicorp/go-plugin)
├── Standard SDK interface: Info(), Discover(), ValidateConfig()
└── Streaming DiscoveryEvent channel
```

## 3. Data Architecture

### 3.1 PostgreSQL Schema (Primary)

```
PostgreSQL 16+
├── Apache AGE Extension
│   ├── knowledge_graph (default graph)
│   │   ├── Vertices: resource nodes with properties
│   │   └── Edges: typed relationships with properties + temporal validity
│   └── Cypher query support
│
├── pgvector Extension
│   ├── embeddings table (id, vector(1536), metadata, source_id)
│   └── IVFFlat index for cosine similarity
│
├── Core Tables
│   ├── resources (id, type, name, provider, region, account_id, properties, tags, discovered_at)
│   ├── relationships (source_id, target_id, type, properties, valid_from, valid_to)
│   ├── discovery_scopes (id, name, plugin, config_encrypted, credential_source, schedule, status)
│   ├── discovery_runs (id, scope_id, status, started_at, completed_at, resources_found, errors)
│   ├── users (id, email, password_hash, roles, created_at, updated_at)
│   ├── audit_entries (id, action, actor, target, details, hash, prev_hash, created_at)
│   ├── api_tokens (id, name, token_hash, created_at, last_used)
│   ├── config (key, value, updated_at)
│   ├── changes (id, resource_id, change_type, before, after, detected_at)
│   ├── documents (id, title, content, doc_type, resource_ids, created_at)
│   ├── runbooks (id, title, content, template, resource_ids, status, created_at)
│   └── evidence (id, type, data, resource_id, created_at)
│
├── Compliance Tables
│   ├── policies (id, name, rego_code, severity, framework, enabled)
│   ├── policy_runs (id, policy_id, status, started_at, completed_at)
│   ├── policy_results (id, run_id, resource_id, passed, details)
│   └── compliance_findings (id, policy_id, resource_id, severity, status, remediation)
│
├── Security Tables (Zerelio Security only)
│   ├── vulnerabilities (id, cve_id, severity, affected_resources, remediation)
│   ├── threat_indicators (id, type, value, confidence, source)
│   ├── security_findings (id, type, resource_id, severity, mitre_technique)
│   └── posture_assessments (id, scope, score, findings_count, assessed_at)
│
└── Migrations (12+ idempotent, SHA-256 checksummed)
```

### 3.2 Data Flow

```
External Sources          Zerelio Knowledge           Consumers
                               |
Cloud APIs ----+     +---------+---------+
  (AWS/Azure/  |     |                   |
   GCP/K8s)   +---->| Discovery Module  |
               |     | (Plugin Manager)  |
SaaS APIs -----+     +--------+----------+
  (GitHub/Jira/        |      |
   Slack/etc.)         v      v
                   Resources + Relationships
                       |
                       v
IaC State ---------> Storage Module
  (Terraform/        (PG + AGE + pgvector)
   Pulumi)             |
                       +---> Graph Store (Cypher)
                       +---> Metadata Store (SQL)
                       +---> Vector Store (embeddings)
                       |
                       v
                   +---+---+
                   | Search | <---- User Queries
                   | Module |       CLI Commands
                   +---+---+       MCP Requests
                       |            API Calls
                       v
                   +---+---+
                   | Docs  | ----> Documents
                   | Module|       Runbooks
                   +---+---+       Architecture Diagrams
                       |
                       v
                   +---+---+
                   |  RAG  | ----> AI Answers
                   |Service|       Confidence Scores
                   +-------+
```

## 4. API Strategy

### 4.1 RESTful API (Primary)

- Base: `/api/v1/`
- Authentication: JWT Bearer tokens (Ed25519 signed)
- Authorization: RBAC with viewer/editor/admin roles
- Rate limiting: configurable per-endpoint
- Pagination: cursor-based for large collections
- Content-Type: application/json
- Versioning: URL path prefix (/api/v1/, /api/v2/)
- OpenAPI 3.1 specification generated from code annotations

### 4.2 MCP (AI Integration)

- JSON-RPC 2.0 protocol
- Transports: stdio (for local AI tools), HTTP (for remote agents)
- Tools: resource search, graph query, doc generation, runbook access
- Resources: resource catalogs, graph schemas, knowledge summaries

### 4.3 gRPC (Plugin Communication)

- Protobuf-defined service: DiscoveryPlugin
- Streaming: server-to-client DiscoveryEvent stream
- Managed by hashicorp/go-plugin framework
- Plugin handshake with version negotiation

### 4.4 Webhooks (Event Notification)

- HMAC-SHA256 signed payloads
- Configurable per-event-type subscriptions
- Retry with exponential backoff
- Dead letter queue for failed deliveries

## 5. Security Architecture

### 5.1 Authentication

- **Primary:** JWT tokens signed with Ed25519 (not HS256)
  - Access tokens: 15-minute expiry
  - Refresh tokens: 7-day expiry with rotation on use
- **SSO:** OIDC (Google, GitHub, Microsoft, custom) + SAML 2.0
- **API tokens:** For programmatic access, hashed with argon2id
- **SCIM v2:** For enterprise identity provisioning

### 5.2 Authorization

- RBAC with three default roles: viewer, editor, admin
- Custom roles with fine-grained permissions
- Permission model: resources:read/write/delete, graph:read/write, discovery:read/run, admin:*
- Role inheritance (admin inherits editor inherits viewer)

### 5.3 Data Protection

- **At rest:** AES-256-GCM encryption for sensitive config fields
- **In transit:** TLS 1.3 minimum
- **Passwords:** argon2id with per-user salts (cost: time=1, memory=64MB, threads=4)
- **Secrets:** Credential broker supports Vault, AWS Secrets Manager, encrypted env
- **Audit:** Hash-chained audit log for tamper detection

### 5.4 Trust Boundaries

```
┌─────────────────────────────────────────────────────┐
│ External Network                                     │
│  ┌──────────────┐                                   │
│  │ TLS Termination (Nginx/Ingress)                  │
│  └──────┬───────┘                                   │
│         │                                           │
│  ┌──────┴───────────────────────────────────────┐   │
│  │ Zerelio Server Process                        │   │
│  │  ┌─────────────┐  ┌──────────────────────┐   │   │
│  │  │ Auth        │  │ API Layer            │   │   │
│  │  │ Middleware   │->│ (rate limited, CORS) │   │   │
│  │  └─────────────┘  └──────────┬───────────┘   │   │
│  │                              │                │   │
│  │  ┌──────────────────────────┐│                │   │
│  │  │ Internal Modules         ││                │   │
│  │  │ (no network boundary)    │<                │   │
│  │  └──────────┬───────────────┘                 │   │
│  │             │                                 │   │
│  └─────────────┼─────────────────────────────────┘   │
│                │                                     │
│  ┌─���───────────┴──────────┐  ┌───────────────────┐  │
│  │ PostgreSQL (TLS)       │  │ Plugin Processes   │  │
│  │ (trusted internal)     │  │ (sandboxed, gRPC)  │  │
│  └────────────────────────┘  └───────────────────┘  │
│                                                      │
│  ┌────────────────────────┐  ┌───────────────────┐  │
│  │ RAG Service (internal) │  │ LLM APIs          │  │
│  │ (no public access)     │  │ (external, TLS)   │  │
│  └────────────────────────┘  └───────────────────┘  │
└─────────────────────────────────────────────────────┘
```

## 6. Deployment Architecture

### 6.1 Minimal (Development / Small Team)

```
Single machine:
  zerelio-server (Go binary)
  PostgreSQL 16 (with AGE + pgvector)
  zerelio-rag (optional, Python)
```

### 6.2 Standard (Production)

```
Kubernetes cluster:
  Deployment: zerelio-server (2+ replicas, HPA)
  Deployment: zerelio-rag (1+ replicas)
  StatefulSet: PostgreSQL (or managed: RDS, Cloud SQL)
  Job: zerelio-discovery (periodic discovery runs)
  DaemonSet: zerelio-agent (optional, for host-level data)
  Ingress: TLS termination, routing
  NetworkPolicy: pod-to-pod restrictions
  ServiceMonitor: Prometheus metrics
```

### 6.3 Helm Chart Values

```yaml
replicaCount: 2
image:
  repository: ghcr.io/zerelio/knowledge
  tag: latest
database:
  host: postgresql
  port: 5432
  extensions:
    - age
    - pgvector
llm:
  provider: openai  # or anthropic, ollama
  model: gpt-4o
discovery:
  pluginDir: /opt/zerelio/plugins
  scanInterval: 15m
```

## 7. Extension Model

### 7.1 Plugin SDK (Primary Extension Point)

Third-party developers extend Zerelio by implementing the `DiscoveryPlugin` interface:

```go
type DiscoveryPlugin interface {
    Info() PluginInfo
    Discover(ctx context.Context, req DiscoverRequest) (<-chan DiscoveryEvent, error)
    ValidateConfig(config map[string]string) ValidationResult
}
```

Plugins are distributed as standalone Go binaries and loaded at runtime.

### 7.2 MCP Tools (AI Extension Point)

AI agents extend Zerelio by connecting via MCP and using exposed tools:
- Search resources, query graph, generate docs
- Custom tool registration for domain-specific operations

### 7.3 Webhook Subscriptions (Event Extension Point)

External systems integrate via webhook subscriptions on events:
- resource.created, resource.updated, resource.deleted
- discovery.started, discovery.completed
- document.generated, runbook.generated
- compliance.violation

### 7.4 API (Integration Extension Point)

Full REST API for programmatic integration with CI/CD, ITSM, monitoring, and alerting platforms.
