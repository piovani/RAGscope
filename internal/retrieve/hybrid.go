package retrieve

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/piovani/RAGscope/internal/domain"
)

// rrfK is the usual constant of Reciprocal Rank Fusion. A bigger value makes
// the first positions count less compared to the following ones.
const rrfK = 60

// Hybrid asks several retrievers and merges their rankings with Reciprocal
// Rank Fusion (RRF): a chunk earns 1/(rrfK + position) from each retriever that
// returned it. Only positions matter, so scores from different retrievers
// (cosine similarity, BM25) never need to be compared.
type Hybrid struct {
	Retrievers []Retriever
}

func (h Hybrid) Name() string { return "hybrid" }

func (h Hybrid) Retrieve(ctx context.Context, query string, k int) ([]domain.ScoredChunk, error) {
	// Fusion works better when each retriever offers more than the final k.
	pool := max(2*k, 20)

	rankings := make([][]domain.ScoredChunk, 0, len(h.Retrievers))
	for _, retriever := range h.Retrievers {
		results, err := retriever.Retrieve(ctx, query, pool)
		if err != nil {
			return nil, fmt.Errorf("%s retriever: %w", retriever.Name(), err)
		}
		rankings = append(rankings, results)
	}
	return fuse(rankings, k), nil
}

// fuse merges rankings with Reciprocal Rank Fusion and returns the top k.
// Ties are broken by chunk ID so the result is always the same.
func fuse(rankings [][]domain.ScoredChunk, k int) []domain.ScoredChunk {
	scores := map[string]float64{}
	chunks := map[string]domain.Chunk{}

	for _, results := range rankings {
		for position, result := range results {
			scores[result.ID] += 1 / float64(rrfK+position+1)
			chunks[result.ID] = result.Chunk
		}
	}

	merged := make([]domain.ScoredChunk, 0, len(chunks))
	for id, chunk := range chunks {
		merged = append(merged, domain.ScoredChunk{Chunk: chunk, Score: scores[id]})
	}

	slices.SortFunc(merged, func(a, b domain.ScoredChunk) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(a.ID, b.ID))
	})
	if len(merged) > k {
		merged = merged[:k]
	}
	return merged
}
