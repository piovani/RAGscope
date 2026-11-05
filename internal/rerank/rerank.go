// Package rerank reorders retrieved candidates with a cross-encoder.
package rerank

import (
	"context"

	"github.com/piovani/RAGscope/internal/domain"
)

// Reranker rescores candidates against the query and keeps the best topN.
type Reranker interface {
	Name() string
	Rerank(ctx context.Context, query string, candidates []domain.ScoredChunk, topN int) ([]domain.ScoredChunk, error)
}
