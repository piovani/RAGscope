package retrieve

import (
	"context"
	"errors"
	"fmt"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/embed"
)

// Searcher finds the stored chunks closest to a vector.
type Searcher interface {
	Search(ctx context.Context, collection string, vector []float32, k int) ([]domain.ScoredChunk, error)
}

// Vector embeds the question and looks for the nearest chunks.
type Vector struct {
	Embedder   embed.Embedder
	Searcher   Searcher
	Collection string
}

func (v Vector) Name() string { return "vector" }

func (v Vector) Retrieve(ctx context.Context, query string, k int) ([]domain.ScoredChunk, error) {
	if k <= 0 {
		return nil, errors.New("k must be greater than zero")
	}

	vectors, err := v.Embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed question: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("expected 1 vector for the question, got %d", len(vectors))
	}

	return v.Searcher.Search(ctx, v.Collection, vectors[0], k)
}
