package retrieve

import (
	"context"
	"fmt"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/rerank"
)

// Reranked takes the candidates from another retriever and lets a cross-encoder
// put them in a better order. The first search is cheap and wide; the reranker
// is slow and precise, so it only sees the candidates.
type Reranked struct {
	Base       Retriever
	Reranker   rerank.Reranker
	Candidates int // how many chunks to fetch from Base before reranking
}

func (r Reranked) Name() string { return r.Base.Name() + "+rerank" }

// Retrieve returns the k best chunks after reranking.
func (r Reranked) Retrieve(ctx context.Context, query string, k int) ([]domain.ScoredChunk, error) {
	candidates, err := r.Base.Retrieve(ctx, query, max(r.Candidates, k))
	if err != nil {
		return nil, err
	}

	reranked, err := r.Reranker.Rerank(ctx, query, candidates, k)
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}
	return reranked, nil
}
