# ADR-004: PostgreSQL + AGE + pgvector as Unified Storage

**Status:** Accepted
**Date:** 2026-11-21
**Decision Makers:** Principal Architect

## Context

A knowledge infrastructure platform needs three types of storage:
1. **Graph storage** for knowledge graph (nodes, edges, traversals, path queries)
2. **Relational storage** for metadata (users, scopes, runs, config, audit)
3. **Vector storage** for semantic search (embeddings, similarity queries)

The existing Knowledge Tree uses PostgreSQL with Apache AGE (graph) and pgvector (vectors), with an in-memory fallback. This decision evaluates whether to keep this approach or adopt specialized databases.

## Decision

**Keep PostgreSQL as the single database with AGE and pgvector extensions.** Make it the mandatory production backend. Keep the in-memory backend only for unit testing.

## Evaluation

### Graph Storage: PostgreSQL + Apache AGE vs Alternatives

| Criteria | PostgreSQL + AGE | Neo4j | ArangoDB | TypeDB |
|----------|-----------------|-------|----------|--------|
| Query language | Cypher (via AGE) | Cypher (native) | AQL | TypeQL |
| Performance (graph traversal) | Good (not native) | Excellent | Good | Good |
| ACID transactions | Full | Full | Full | Full |
| Relational queries | Native SQL | Limited | Limited | None |
| Operational complexity | Low (one DB) | Medium (new DB) | Medium | High |
| Self-hosting ease | Excellent | Good | Good | Fair |
| License | Apache-2.0 | GPL/Commercial | Apache-2.0 | AGPL/Commercial |
| Ecosystem | Massive | Large | Medium | Small |
| Extension model | SQL + Cypher | Cypher only | AQL only | TypeQL only |

**Decision rationale:**
- Adding Neo4j means operating TWO databases. The operational overhead for self-hosters is significant.
- Apache AGE provides Cypher query support within PostgreSQL, good enough for our graph patterns.
- We avoid vendor lock-in — AGE is Apache-2.0, PostgreSQL is PostgreSQL License (permissive).
- Graph-specific performance bottlenecks can be addressed with materialized views and indexes.
- Neo4j's Community Edition is GPL, which creates licensing complications for our Apache-2.0 project.

**Trade-off accepted:** AGE's Cypher support is less mature than Neo4j's. Complex graph algorithms (community detection, betweenness centrality) may need custom implementation rather than calling a library function.

### Vector Storage: pgvector vs Alternatives

| Criteria | pgvector | Milvus | Qdrant | Weaviate | Chroma |
|----------|----------|--------|--------|----------|--------|
| Operational complexity | None (PG extension) | High | Medium | Medium | Low |
| Performance (ANN) | Good (IVFFlat, HNSW) | Excellent | Excellent | Good | Fair |
| Scale | Millions of vectors | Billions | Billions | Millions | Millions |
| Filtering | Full SQL WHERE | Limited | Good | Good | Basic |
| Transactions | ACID | Eventually consistent | No txn | No txn | No txn |
| Self-hosting | Part of PG | Complex | Simple | Medium | Simple |

**Decision rationale:**
- pgvector eliminates the need for a separate vector database service.
- For the expected scale (thousands to low millions of embeddings), pgvector with HNSW indexing is sufficient.
- ACID transactions mean we can update resources and their embeddings atomically.
- If we ever need billion-scale vector search, we can add Qdrant as an optional backend without changing the interface.

### Embedding Model Strategy

| Model | Dimensions | Quality (MTEB) | Speed | Cost |
|-------|-----------|-----------------|-------|------|
| OpenAI text-embedding-3-small | 1536 | Good | Fast | $0.02/1M tokens |
| OpenAI text-embedding-3-large | 3072 | Excellent | Fast | $0.13/1M tokens |
| Cohere embed-v3 | 1024 | Good | Fast | $0.10/1M tokens |
| all-MiniLM-L6-v2 (local) | 384 | Fair | Very fast | Free |
| nomic-embed-text (local) | 768 | Good | Fast | Free |

**Default:** OpenAI text-embedding-3-small (1536d) for cloud deployments.
**Self-hosted:** nomic-embed-text via Ollama for air-gapped environments.
**Configurable:** Users choose their embedding provider via config.

## Consequences

**Positive:**
- Single database to operate, back up, monitor, and secure
- ACID transactions across graph, relational, and vector operations
- Familiar PostgreSQL ecosystem (tooling, hosting, expertise)
- No vendor lock-in on any storage component
- Simpler deployment for self-hosters

**Negative:**
- Graph query performance ceiling lower than native graph databases
- Vector search performance ceiling lower than dedicated vector DBs
- Apache AGE is less mature than Neo4j (smaller community, fewer features)
- pgvector HNSW index build is slower than dedicated vector DBs

**Mitigation:** Design the storage interfaces so that specialized backends (Neo4j, Qdrant) can be added as optional alternatives in the future without changing the application logic.

## Alternatives Considered

1. **Neo4j + PostgreSQL + Qdrant (three databases):** Maximum performance per category, but triple the operational complexity. Rejected for an OSS project where self-hosting ease is competitive advantage.
2. **ArangoDB (multi-model):** Graph + document + search in one database. Interesting but smaller ecosystem and we'd lose PostgreSQL's maturity.
3. **SQLite for development:** Considered for zero-config development. Rejected because AGE and pgvector don't have SQLite equivalents, so the dev/prod behavior divergence would be too large.
