// Package regression is a CI gate that needs no services.
//
// It protects the deterministic part of the pipeline: the chunkers, the
// tokenizer, BM25, the metric code and the golden set. If a change to any of
// them makes lexical retrieval clearly worse on the real corpus, this fails.
//
// It does NOT measure embeddings or model quality. Those need Ollama and
// Postgres and are compared with real experiment runs, not here.
//
// Measured values (k=5), all deterministic:
//
//	fixed-512/64       recall@5 = 0.8175  MRR = 0.6439
//	structural-512/64  recall@5 = 0.6632  MRR = 0.5747
//
// Thresholds sit about 0.05 below, so only a real regression fails.
package regression

import (
	"context"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/eval"
	"github.com/piovani/RAGscope/internal/ingest"
	"github.com/piovani/RAGscope/internal/retrieve"
)

const topK = 5

type chunker interface {
	Chunk(doc domain.Document) ([]domain.Chunk, error)
}

func TestRetrievalQuality(t *testing.T) {
	docs, err := ingest.LoadDir("../../data/raw")
	if err != nil {
		t.Fatal(err)
	}
	questions, err := eval.LoadQuestions("../../data/golden/questions.yaml")
	if err != nil {
		t.Fatal(err)
	}

	// The golden set must match the corpus, or every score below is meaningless.
	if missing := eval.MissingEvidence(questions, docs); len(missing) > 0 {
		t.Fatalf("evidence not found in the corpus: %v", missing)
	}

	tests := []struct {
		name      string
		chunker   chunker
		minRecall float64
		minMRR    float64
	}{
		{"fixed-512/64", ingest.Fixed{Size: 512, Overlap: 64}, 0.77, 0.59},
		{"structural-512/64", ingest.Structural{MaxWords: 512, Overlap: 64}, 0.61, 0.52},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var chunks []domain.Chunk
			for _, doc := range docs {
				c, err := tc.chunker.Chunk(doc)
				if err != nil {
					t.Fatal(err)
				}
				chunks = append(chunks, c...)
			}

			report, err := eval.RunRetrieval(context.Background(), retrieve.NewBM25(chunks), questions, topK)
			if err != nil {
				t.Fatal(err)
			}

			recall := report.Final().Recall
			t.Logf("recall@%d=%.4f mrr=%.4f", topK, recall, report.MRR)
			if recall < tc.minRecall {
				t.Errorf("recall@%d = %.4f, want at least %.2f", topK, recall, tc.minRecall)
			}
			if report.MRR < tc.minMRR {
				t.Errorf("MRR = %.4f, want at least %.2f", report.MRR, tc.minMRR)
			}
		})
	}
}
