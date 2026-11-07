# Methodology

How the numbers in this project are produced, and how far they can be trusted.

## Corpus

The corpus is the LGPD (Lei nº 13.709/2018), the Brazilian data protection law.
It is public text, in Portuguese, and it is organized in articles, paragraphs and
items, which makes chunking a real question.

The text comes from the compiled version published by the Planalto portal.
That page shows revoked wording struck through. Copying it as plain text loses the
strike and mixes old and current versions: the same item of art. 5º appeared five
times. So the original page is kept in `data/sources/` and `ragscope convert`
rebuilds `data/raw/lgpd.txt` from it, dropping the struck-through parts. The
result has all 65 articles and one wording per item. See [data/README.md](../data/README.md).

## Golden set

`data/golden/questions.yaml` has 105 questions: 95 that the law answers and 10
that it does not (`unanswerable`). The system must refuse those.

### Evidence is text, not a chunk id

Each answerable question carries one or more **literal snippets of the law**. A
retrieved chunk counts as relevant if it contains a snippet (case and spacing are
ignored). Chunk ids change with every chunking strategy, snippets do not, so the
same golden set serves every experiment.

Consequences to keep in mind:

- When a question lists several snippets, **all** of them are required. There is no
  "any of these is fine".
- A snippet cut in half by a chunk border does not count. Snippets are kept short
  to make this unlikely, but it can happen.
- Every snippet must occur exactly once in the corpus. A repeated snippet would make
  unrelated chunks look relevant. `ragscope golden` checks that each snippet exists;
  uniqueness was checked with a script when the questions were written.
- A question can be answered by a passage that is not listed as evidence. That
  passage is counted as a miss.

### How it was written

The questions were drafted by an AI model from the law text, in three batches:

| Batch | Questions | Style |
|---|--:|---|
| 1 | 34 | close to the wording of the articles |
| 2 | 38 | everyday language ("Posso pedir uma cópia eletrônica de tudo que a empresa tem sobre mim?") |
| 3 | 33 | articles that still had no question |

Each of batches 2 and 3 was reviewed by a **separate, independent agent** that read
the law before reading the reference answer. It found problems in about a third of
the questions (13 of 38 and 12 of 33): answers stronger than the law ("Sim" where
the law says "poderá"), exceptions left out, evidence that did not cover the whole
answer, and one factual error. The fixes were checked against the law text before
being applied.

**This does not remove the bias of having a model write the questions.** A sample
reviewed by a person is still missing.

## Metrics

### Retrieval (`ragscope eval`)

No model is involved, so these are cheap and repeatable. They look at the first
`k` retrieved chunks of each answerable question.

| Metric | Meaning |
|---|---|
| Hit rate@k | share of questions with at least one relevant chunk in the top k |
| Recall@k | average share of a question's evidence snippets found in the top k |
| MRR | average of 1 / position of the first relevant chunk (0 if none) |

### Answers (`ragscope eval-answers`)

| Metric | Meaning |
|---|---|
| Correct refusal | share of unanswerable questions the model refused |
| False refusal | share of answerable questions the model refused |
| Citation rate | share of real answers that cite a passage like `[1]` |
| Faithfulness | share of answers a judge model found supported by the passages (optional, `--judge`) |

Refusal is detected with a fixed marker word (`SEM_RESPOSTA`) that the prompt asks
the model to write, which is more reliable than reading a polite sentence.

Not measured yet: whether the answer is **correct** compared with the reference
answer, and cost in money. Faithfulness is not correctness: an answer can be fully
supported by a passage and still answer a different question. This happened in the
first sample, when the model defined "pseudonimização" for a question about
"anonimização".

The faithfulness judge is the same small model that writes the answers, so it is a
weak judge. Its scores are labeled "not calibrated" and must be compared with
human grades before they are quoted.

## Statistics

Comparing two experiments by their averages is not enough with this few questions.
`ragscope report` compares each experiment with the baseline **question by question**
and draws a 95% confidence interval for the mean difference with a paired bootstrap
(5000 resamples, fixed seed, so the same data always gives the same interval). A
difference is starred when its interval does not include zero.

Limits of this method:

- The bootstrap treats questions as independent, but several come from the same
  article. The real uncertainty is probably larger than the interval shows.
- An interval that includes zero does not prove there is no difference. It says
  that this many questions cannot tell.
- Every run saves a fingerprint of the golden set. `report` warns when it compares
  results that were scored on different versions of it.

## Latency

Latency numbers saved with the results are **not reliable** for the reranker. They
were measured on an 8 GB Mac with the swap almost full, while Docker, Ollama and
the reranker competed for memory. Treat them as an upper bound until they are
measured again on a quiet machine.

## Reproducing the results

Run `make build` first. The commands below call `ragscope` as a shortcut for `./bin/ragscope`.

```bash
make up                          # Postgres with pgvector
make models                      # Ollama models: bge-m3 and llama3.2:3b
make corpus                      # optional: rebuild data/raw/lgpd.txt from data/sources

ragscope golden                  # every evidence snippet exists in the corpus
ragscope ingest -c configs/experiments/baseline.yaml              # fixed chunks
ragscope ingest -c configs/experiments/ablation-structural.yaml   # one chunk per article

for e in baseline ablation-bm25 ablation-hybrid ablation-structural ablation-hybrid-structural; do
  ragscope eval -c configs/experiments/$e.yaml
done

make rerank                      # in another terminal, for the reranker experiments
for e in ablation-rerank hybrid-rerank-structural; do
  ragscope eval -c configs/experiments/$e.yaml
done

ragscope report                  # each experiment's own k
ragscope report --k 1            # only the first result
```

The first `make rerank` downloads the reranker model (about 2 GB).
