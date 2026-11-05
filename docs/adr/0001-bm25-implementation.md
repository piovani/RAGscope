# ADR 0001 — BM25 implementation

**Status:** Proposed

## Context

Hybrid retrieval needs a lexical ranker next to vector search. Two options:

1. **In-process Go BM25** over chunks loaded from Postgres.
2. **Postgres-side ranking** (full-text search `ts_rank`, or an extension that implements true BM25).

## Options

| | In-process Go | Postgres-side |
|---|---|---|
| Fidelity to BM25 | Exact | `ts_rank` is not BM25; extension needed for exact |
| Operational cost | Index rebuilt/loaded at startup | Lives next to the vectors, one datastore |
| Portuguese stemming | Must implement or import | Built-in `portuguese` config |
| Experiment flexibility (tune k1, b) | Easy | Harder |

## Decision

_Pending._ Prototype both during Phase 3 and compare recall@k and latency on the golden set before choosing.

## Consequences

The `retrieve.Retriever` interface hides the choice, so the decision can be reversed without touching the pipeline.
