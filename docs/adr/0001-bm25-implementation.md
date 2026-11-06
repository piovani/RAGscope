# ADR 0001 — BM25 implementation

**Status:** Accepted (revisit if the corpus grows or recall on exact terms is poor)

## Context

Hybrid retrieval needs a lexical ranker next to vector search. Two options:

1. **In-process Go BM25** over chunks loaded from Postgres.
2. **Postgres-side ranking** (full-text search `ts_rank`, or an extension that implements true BM25).

## Options

| | In-process Go | Postgres-side |
|---|---|---|
| Fidelity to BM25 | Exact | `ts_rank` is not BM25; extension needed for exact |
| Operational cost | Index built in memory at startup | Lives next to the vectors, one datastore |
| Portuguese stemming | None (accents and stopwords only) | Built-in `portuguese` config |
| Experiment flexibility (tune k1, b) | Easy | Harder |

## Decision

In-process Go BM25 (`internal/retrieve/bm25.go`). The corpus is a few thousand
chunks, so building the index takes milliseconds, and we get the real BM25
formula with tunable `k1` and `b`.

## Consequences

- No stemming: "dado" and "dados" are different terms. This probably lowers
  BM25 recall on Portuguese. If the ablation shows it matters, a Portuguese
  stemmer or the Postgres option is the next thing to try.
- The index is rebuilt on every `eval`/`ask` run from the stored chunks.
- The `retrieve.Retriever` interface hides the choice, so it can change
  without touching the pipeline.
