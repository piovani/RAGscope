// Package retrieve finds the chunks most relevant to a question.
package retrieve

import (
	"context"

	"github.com/piovani/RAGscope/internal/domain"
)

// Retriever returns the top-k chunks most relevant to a query.
type Retriever interface {
	Name() string
	Retrieve(ctx context.Context, query string, k int) ([]domain.ScoredChunk, error)
}
