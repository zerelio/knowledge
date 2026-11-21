# Industry Landscape and Technology Evaluation

**Date:** 2026-11-21
**Research Phase:** Phase One

## Strategic Positioning

The core strategic positioning for Zerelio: **own the retrieval and memory primitives** (graph+vector hybrid, infrastructure-aware knowledge retrieval), expose them through typed pipeline interfaces, and avoid competing at the LLM orchestration layer where LangChain/LlamaIndex already dominate. The gap across all existing infrastructure discovery tools is AI-powered knowledge retrieval layered on infrastructure topology data.

## Key Findings

### Competitor Landscape

**Graph Databases:**
- Neo4j: ecosystem leader, 65+ algorithms in GDS, but Community Edition is GPLv3 (single-instance only), commercial starts ~$36K/year
- Apache AGE: openCypher on PostgreSQL, Apache-2.0, eliminates need for separate graph DB
- TypeDB: most principled type system (hypergraph + inference), but high-risk (small community, recent rewrite)
- Dgraph: strongest OSS distributed graph DB (Apache-2.0, Go), but turbulent corporate history

**Infrastructure Discovery:**
- Backstage (34.9k stars): CNCF standard for developer portals, plugin architecture (framework, not turnkey)
- CloudQuery (6.6k stars): ETL with gRPC/protobuf plugin model, 70+ source plugins, 1M rows/2s
- Steampipe (8k stars): zero-ETL, SQL over cloud APIs via FDW (AGPL-3.0)
- Cartography (4.2k stars, CNCF Sandbox): infrastructure graph in Neo4j, security traversal focus
- Gap: none combine discovery + graph + AI retrieval in one platform

**RAG/Knowledge Retrieval:**
- LightRAG (40.8k stars): graph+vector dual-level retrieval, lower cost than GraphRAG (EMNLP 2025)
- GraphRAG (36.7k stars): hierarchical community structures, now in maintenance mode
- Cognee (33.4k stars): local-first, GLiNER extraction, three-stage memory lifecycle
- Haystack (27.1k stars): cleanest extensibility model (DAG pipelines with typed components)
- Convergence: graph+vector hybrid retrieval is becoming table stakes

**AI Memory:**
- Mem0 (69.3k stars): market leader, ADD-only extraction, multi-signal retrieval, 92.5 on LoCoMo
- Letta/MemGPT (25.5k stars): self-managing context with tiered memory

### Technology Recommendations

| Decision | Recommendation | Rationale |
|----------|---------------|-----------|
| Graph storage | PostgreSQL + Apache AGE | Unified stack, no GPL complications |
| Vector storage | pgvector (start), Qdrant (scale) | Co-location; Qdrant when >5M vectors |
| Embeddings | OpenAI (bootstrap), BGE/E5 (production) | Privacy and cost at scale |
| Core language | Go + Python sidecars | Cloud-native ecosystem; AI/ML via gRPC |
| Architecture | Modular monolith | Data consistency, team-size appropriate |
| OSS/commercial | Apache 2.0 + directory separation | Clean separation without community fracture risk |
| Multi-tenancy | PostgreSQL RLS + DB-per-tenant premium | Low overhead default; compliance upgrade |

### Plugin Architecture Note

The research agent recommended WASM-based plugins (Extism + WIT). However, Knowledge Tree already has a working hashicorp/go-plugin system with 22 plugins. **Decision: keep hashicorp/go-plugin for Phase Four implementation.** WASM migration is a future consideration when multi-language plugin support becomes a priority. The existing gRPC-based process isolation already provides sandboxing.

### Lessons from Industry

1. **HashiCorp BSL transition spawned OpenTofu (33K stars):** Never retrofit OSS/commercial separation. Design it clean from day one.
2. **Shopify modular monolith (~300 modules via Packwerk):** Teams under ~50 engineers that split into microservices prematurely hit the "distributed monolith" anti-pattern.
3. **Neo4j GPLv3 Community Edition:** GPL contamination prevents embedding in Apache-2.0 projects. PostgreSQL + AGE avoids this.
4. **GraphRAG in maintenance mode:** Creates opening for better graph-enhanced retrieval. LightRAG dual-level approach is the successor pattern.
