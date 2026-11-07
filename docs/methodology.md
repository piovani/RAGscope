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
| False refusal | share of answerable questions the model refused, split by cause (below) |
| Citation rate | share of real answers that cite a passage like `[1]` |
| Correct | share of answerable questions answered like the reference answer, judged by a model (`--correctness`). A refusal counts as wrong. |
| Faithfulness | share of answers a judge model found supported by the passages (`--faithfulness`) |
| Cost | dollars spent, for hosted models only. Local models cost zero. |

Refusal is detected with a fixed marker word (`SEM_RESPOSTA`) that the prompt asks
the model to write, which is more reliable than reading a polite sentence.

**A refusal is split by cause.** For each answerable question the run records
whether all the evidence snippets were among the passages the model saw. If they
were not, refusing was the fair thing to do and the retriever is to blame. If they
were, the model refused with the answer in front of it. Counting both together
hides which part of the system to fix.

**Correct and faithful are different questions.** An answer can be fully supported
by a passage and still answer something else. This happened in the first sample,
when the model defined "pseudonimização" for a question about "anonimização". So
correctness is checked against the reference answer, with a short prompt that does
not need the passages, and faithfulness is checked against the passages, with a
longer prompt that is much slower.

### The judge is a model, so it is checked against a person

The judge is the same small model that writes the answers, because a bigger one
does not fit in memory. That is a weak judge, so its scores stay labeled "not
calibrated" until a person has graded a sample:

1. `ragscope grade-sheet` picks about 30 real answers to answerable questions,
   evenly spaced so the choice is not random, and writes a YAML file. Each entry
   has the question, the reference answer, the model's answer and the passages.
   **It does not show the judge's verdicts**, so the person is not anchored by them.
2. The person fills `human_correct` (required) and, if there is time,
   `human_supported`.
3. `ragscope calibrate` compares those grades with the judge's: percent agreement,
   Cohen's kappa (agreement beyond chance), the 2x2 table, the judge's false
   positives and false negatives, and the list of disagreements. With fewer than 20
   entries compared it warns that the sample is too small to trust.

Until step 2 is done, no judge number should be quoted as a result.

## Query rewriting

Two rewrites change the question before the search. Both use the experiment's own
model, so a run is not mixed with a second one, and their calls are not counted in
the cost of the answers.

- **HyDE.** The model writes a short hypothetical passage, in the style of a law
  article, that would answer the question. The search uses the question plus that
  passage, which tends to be closer to the wording of the law than the question is.
  If the model fails or returns nothing, the run fails. It never falls back to the
  plain question, because that would hide the failure in the numbers.
- **Multi-query.** The model rephrases the question a few times, each version is
  searched and the rankings are merged with Reciprocal Rank Fusion. A chunk found by
  several versions beats one found by a single version.

## Hosted model

The adapter for the Anthropic API exists and is tested against a fake server, but
**no run with a hosted model is part of the results**: it needs an API key and
money. The key is read only from the `ANTHROPIC_API_KEY` environment variable, and
the experiment must state the prices per million tokens, so the cost can never be
zero by accident. The example is `configs/experiments/hosted/ablation-claude.yaml`,
kept out of the folder that CI validates.

## Regression gate

CI cannot run Ollama or a reranker, so it protects the parts that do not need them.
A test chunks the real corpus with both chunkers, searches it with BM25 and fails
if recall@5 or MRR drops below a threshold set a little under the measured value.
That guards the chunkers, the tokenizer, BM25, the metric code and the golden set
(every evidence snippet must still exist in the corpus). It does not measure
embeddings or model quality. The store integration test runs against a real
pgvector database in CI.

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

# query rewriting (uses the model during retrieval)
for e in ablation-hyde ablation-multiquery; do
  ragscope eval -c configs/experiments/$e.yaml
done

# answers: refusals, citations and a correctness judge
ragscope eval-answers -c configs/experiments/baseline.yaml --correctness
# add --faithfulness to also judge support by the passages (slow), --limit N for a sample

# calibrate the judge with your own grades
ragscope grade-sheet --answers experiments/results/generation/<file>.json --out data/golden/grading/sheet.yaml
#   ...fill in human_correct in the sheet...
ragscope calibrate  --answers experiments/results/generation/<file>.json --sheet data/golden/grading/sheet.yaml
```

The first `make rerank` downloads the reranker model (about 2 GB).
