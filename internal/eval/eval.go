// Package eval holds metrics, the LLM judge and the experiment runner.
package eval

import (
	"context"

	"github.com/piovani/RAGscope/internal/domain"
)

// Sample is one golden-set question together with what the pipeline produced for it.
type Sample struct {
	ID              string
	Question        string
	ReferenceAnswer string
	GoldChunkIDs    []string
	Answerable      bool // false for questions whose answer is not in the corpus

	// Filled in by the runner after executing the pipeline.
	Retrieved []domain.ScoredChunk
	Answer    string
	Refused   bool
}

// Metric scores a single Sample in [0, 1].
type Metric interface {
	Name() string
	Score(ctx context.Context, s Sample) (float64, error)
}
