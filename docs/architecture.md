# Architecture

This is how RAGscope works today, from the law text to the comparison table.
The same diagram is in the [README](../README.md). If you change one, change both.

```mermaid
flowchart TB
    CFG["Experiment config<br/>configs/experiments/*.yaml"]

    subgraph ING["1. Ingestion (offline)"]
        SRC["Law page as published<br/>data/sources/*.html"] -->|"ragscope convert<br/>drops revoked text"| TXT["Clean text<br/>data/raw/*.txt"]
        TXT -->|"ragscope ingest"| CH{"Chunking"}
        CH -->|"fixed"| CHK["Chunks"]
        CH -->|"structural<br/>one per article"| CHK
        CHK --> EMB["Embeddings<br/>Ollama, bge-m3"]
        EMB --> PG[("Postgres + pgvector<br/>one collection per index")]
    end

    subgraph QRY["2. Query (ragscope ask and eval)"]
        Q["Question"] --> MODE{"Retrieval mode"}
        MODE -->|"vector"| VEC["Cosine search<br/>in pgvector"]
        MODE -->|"bm25"| BM["BM25<br/>in memory"]
        MODE -->|"hybrid"| HYB["Vector + BM25<br/>merged with RRF"]
        VEC --> TOP["Top-k passages"]
        BM --> TOP
        HYB --> TOP
        TOP -->|"without reranker"| PROMPT
        TOP -->|"with reranker"| RR["Reranker<br/>bge-reranker-v2-m3"]
        RR --> PROMPT["Prompt<br/>numbered passages, cite as [n],<br/>SEM_RESPOSTA to refuse"]
        PROMPT --> LLM["LLM<br/>Ollama, llama3.2:3b"]
        LLM --> ANS["Answer with sources<br/>or a refusal"]
    end

    subgraph EVAL["3. Evaluation"]
        GOLD[("Golden set<br/>105 questions<br/>data/golden/questions.yaml")]
        GOLD -->|"every question"| Q
        TOP -->|"passages scored"| EV["ragscope eval<br/>hit rate, recall at k, MRR"]
        ANS -->|"answers scored"| EA["ragscope eval-answers<br/>refusals, citations, faithfulness"]
        EV --> RES[("JSON results<br/>experiments/results")]
        EA --> RES
        RES --> REP["ragscope report<br/>table with 95% CI vs baseline"]
    end

    PG --> VEC
    PG -->|"chunks loaded<br/>at query time"| BM
    CFG -.->|"chunking, embedding model"| CH
    CFG -.->|"mode, k, reranker, generator"| MODE
```

## What each stage does

| Stage | What it does | Code |
|---|---|---|
| `convert` | Reads the saved law page and keeps only the current wording. The page shows revoked text struck through, and a plain copy and paste would mix old and new versions. | `internal/ingest/html.go` |
| `ingest` | Cuts the text into chunks, embeds them and stores them. Experiments that only differ in retrieval, reranking or generation share one index. | `internal/ingest`, `internal/embed`, `internal/store` |
| Retrieval | Finds the chunks closest to the question: by meaning (vector), by words (BM25) or both merged with Reciprocal Rank Fusion. | `internal/retrieve` |
| Reranker | A cross-encoder re-scores the candidates and keeps the best ones. It is the only stage that reads the question and the passage together. | `internal/rerank`, `scripts/rerank_server.py` |
| Generation | Builds a prompt with numbered passages and asks the model to answer only from them, to cite them, and to say `SEM_RESPOSTA` when they do not contain the answer. | `internal/generate` |
| `eval` | Checks, for each golden question, where the evidence snippet shows up in the retrieved passages. No model is involved, so it is cheap and repeatable. | `internal/eval` |
| `eval-answers` | Generates the answers and scores refusals, citations and (optionally) faithfulness judged by a model. | `internal/eval` |
| `report` | Compares the saved results and says whether a difference is bigger than luck. | `internal/report` |

## Decisions worth knowing

- **Every stage is an interface** (`Chunker`, `Embedder`, `Retriever`, `Reranker`, `Generator`), so an experiment is only a different wiring of the same pieces.
- **A collection is named after what changes the index** (corpus, chunking, embedding model), not after the experiment. That is why `baseline`, `ablation-bm25` and `ablation-hybrid` need a single `ingest`.
- **The golden set points to text, not to chunks.** Each question carries a literal snippet of the law, and a chunk counts as relevant if it contains it. Chunk ids change with every chunking strategy, snippets do not.
- **Vector search is exact**, with no ANN index. For a few thousand chunks it is fast, and it keeps the recall numbers free of index approximation.
- **BM25 lives in memory**, built from the stored chunks. See [ADR 0001](adr/0001-bm25-implementation.md).
- **Embeddings and answers run on Ollama**, on the host, so they can use the Mac GPU. The reranker is a small local Python server on Apple Silicon, because the TEI image only exists for x86.
