package rerank

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/httpjson"
)

// batchSize is the server's default limit of texts per request.
const batchSize = 32

// TEI scores candidates with a cross-encoder served by Text Embeddings Inference.
type TEI struct {
	client *httpjson.Client
}

func NewTEI(url string) *TEI {
	return &TEI{client: httpjson.New(url)}
}

func (t *TEI) Name() string { return "tei" }

func (t *TEI) Rerank(ctx context.Context, query string, candidates []domain.ScoredChunk, topN int) ([]domain.ScoredChunk, error) {
	reranked := make([]domain.ScoredChunk, 0, len(candidates))

	// The cross-encoder scores each text on its own, so scores from different
	// batches can be compared and sorted together.
	for start := 0; start < len(candidates); start += batchSize {
		end := min(start+batchSize, len(candidates))
		batch := candidates[start:end]

		scores, err := t.score(ctx, query, batch)
		if err != nil {
			return nil, err
		}
		for i, candidate := range batch {
			candidate.Score = scores[i]
			reranked = append(reranked, candidate)
		}
	}

	slices.SortStableFunc(reranked, func(a, b domain.ScoredChunk) int {
		return cmp.Compare(b.Score, a.Score) // highest first
	})
	if len(reranked) > topN {
		reranked = reranked[:topN]
	}
	return reranked, nil
}

// score returns one relevance score per candidate, in the same order.
func (t *TEI) score(ctx context.Context, query string, candidates []domain.ScoredChunk) ([]float64, error) {
	texts := make([]string, len(candidates))
	for i, c := range candidates {
		texts[i] = c.Text
	}

	request := map[string]any{"query": query, "texts": texts, "truncate": true}

	// TEI answers with one entry per text, naming which text it scored.
	var answer []struct {
		Index int     `json:"index"`
		Score float64 `json:"score"`
	}
	if err := t.client.Post(ctx, "/rerank", request, &answer); err != nil {
		return nil, err
	}
	if len(answer) != len(texts) {
		return nil, fmt.Errorf("sent %d texts but got %d scores", len(texts), len(answer))
	}

	scores := make([]float64, len(texts))
	for _, item := range answer {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("reranker returned an unknown text index %d", item.Index)
		}
		scores[item.Index] = item.Score
	}
	return scores, nil
}
