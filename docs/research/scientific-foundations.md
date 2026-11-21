# Scientific Foundations for Knowledge Infrastructure

**Date:** 2026-11-21
**Research Phase:** Phase One

## Key Architectural Decisions Informed by Research

| Decision | Research Basis |
|----------|---------------|
| Labeled property graph as primary model | Angles (2018); metadata-rich edges essential for infrastructure |
| Event-based temporal annotation | Leblay & Chekol (2018); Lacroix et al. (2020) |
| LightRAG-style dual-level graph retrieval | Guo et al. (2024); 10-100x more efficient than GraphRAG |
| Three-signal hybrid retrieval (BM25 + dense + graph) | Luan et al. (2021); Ma et al. (2024) |
| ColBERT for re-ranking | Khattab & Zaharia (2020); best quality-latency trade-off |
| Structure-aware chunking | Kamradt (2023); Gunther et al. (2024) |
| Hierarchical memory (observe/reflect/plan) | Park et al. (2023); Packer et al. (2023) |
| RAGAS for production monitoring | Es et al. (2024) |
| Incremental graph updates | Zhong et al. (2023); Zhu et al. (2022) |
| W3C PROV for provenance | Buneman et al. (2001); Moreau & Groth (2013) |

## 1. Knowledge Representation

- **Labeled Property Graphs (LPG) over RDF** for the primary model: LPG allows key-value properties on both nodes and edges natively, critical for infrastructure where relationships carry rich metadata (timestamps, versions, health status). RDF's triple model requires reification for edge properties. (Angles, 2018)
- **Bottom-up domain ontology** following "middle-out" approach: start from TOSCA standard concepts but extend based on observed data patterns rather than top-down standards. (Sabou et al., 2019)
- **Event-based temporal annotation** over versioned snapshots: annotate edges with valid-time intervals [t_start, t_end) for efficient temporal queries while materializing periodic snapshots for compliance. (Leblay & Chekol, 2018)
- **Entity resolution** using collective ER (graph-structure-aware) for infrastructure entities: topology provides strong disambiguating signals. (Christophides et al., 2021; Bhattacharya & Getoor, 2007)

## 2. Graph-Based Retrieval

- **LightRAG dual-level retrieval** (low-level entity/relationship + high-level topic) is 10-100x more efficient than Microsoft's GraphRAG while achieving comparable quality. Avoids expensive community summarization. (Guo et al., 2024)
- **GraphRAG community detection** useful for global sensemaking but indexing cost is high (1M+ LLM tokens per run). Better suited for periodic batch enrichment than real-time retrieval. (Edge et al., 2024)
- **RAPTOR tree-structured summarization** improves multi-step reasoning by 20% over standard RAG. Applicable to hierarchical infrastructure documentation. (Sarthi et al., 2024)
- **HippoRAG Personalized PageRank** particularly relevant for infrastructure troubleshooting: multi-hop reasoning across related entities. (Gutierrez et al., 2024)

## 3. Hybrid Information Retrieval

- **Three-signal hybrid retrieval** (BM25 + dense bi-encoder + graph proximity) consistently outperforms any single approach. BM25 catches exact technical terms (error codes, config params); dense catches semantic similarity; graph catches structural relevance. (Luan et al., 2021; Ma et al., 2024)
- **ColBERT late interaction** for re-ranking: near cross-encoder quality at 100-1000x lower latency. (Khattab & Zaharia, 2020)
- **Structure-aware chunking** that respects document hierarchy (sections, code blocks, config stanzas) outperforms fixed-size by 15-20%. (Kamradt, 2023)
- **"Lost in the middle" effect**: LLMs attend most to beginning and end of context. Context ordering matters as much as selection. (Liu et al., 2024)

## 4. Context Engineering and Memory

- **Hierarchical memory architecture** (observation/reflection/planning) maps naturally to infrastructure: observations = raw events, reflections = incident analyses, plans = runbooks. (Park et al., 2023)
- **Virtual memory analogy** (MemGPT): explicit management policies for paging between limited LLM context and unlimited storage. (Packer et al., 2023)
- **2-5x prompt compression** achievable with <2% quality loss for structured data. Schema-aware compression preserves structure. (Jiang et al., 2023)
- **RAGAS metrics** (faithfulness, relevance, context precision, context recall) for continuous monitoring of retrieval quality. (Es et al., 2024)

## 5. Knowledge Lifecycle

- **Relation-type-specific freshness** policies anchored to monitoring signals, not just time-based expiration. (Dong et al., 2014; Fetahu et al., 2015)
- **Incremental graph updates** achieve 95%+ consistency with full re-extraction at 10-20x lower cost. (Zhong et al., 2023)
- **AnyBURL rule learning** for explainable knowledge completion: learns logical rules from graph structure, interpretable for infrastructure domain. (Meilicke et al., 2019)
- **Source-hierarchy conflict resolution**: direct observation > monitoring > documentation > manual entry. (Dong et al., 2015)
- **W3C PROV provenance tracking** on all knowledge claims for trust assessment and compliance. (Buneman et al., 2001)

## Selected References

- Angles, R. (2018). The Property Graph Database Model. AMW.
- Buneman, P. et al. (2001). Why and Where: A Characterization of Data Provenance. ICDT.
- Christophides, V. et al. (2021). End-to-End Entity Resolution for Big Data. ACM Computing Surveys.
- Edge, D. et al. (2024). From Local to Global: A Graph RAG Approach. Microsoft Research.
- Es, S. et al. (2024). RAGAS: Automated Evaluation of RAG. EACL.
- Guo, Z. et al. (2024). LightRAG: Simple and Fast RAG. University of Hong Kong.
- Gutierrez, B. et al. (2024). HippoRAG. NeurIPS.
- Hogan, A. et al. (2021). Knowledge Graphs. ACM Computing Surveys.
- Khattab, O. & Zaharia, M. (2020). ColBERT. SIGIR.
- Leblay, J. & Chekol, M.W. (2018). Deriving Validity Time in Knowledge Graphs. WWW.
- Liu, N. et al. (2024). Lost in the Middle. TACL.
- Luan, Y. et al. (2021). Sparse, Dense, and Attentional Representations. TACL.
- Packer, C. et al. (2023). MemGPT. arXiv.
- Park, J.S. et al. (2023). Generative Agents. UIST.
- Sarthi, P. et al. (2024). RAPTOR. ICLR.
