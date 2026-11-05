package pipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/embed"
	"github.com/piovani/RAGscope/internal/ingest"
)

// Indexer saves chunks and their vectors, replacing what the collection had.
type Indexer interface {
	Replace(ctx context.Context, collection string, chunks []domain.Chunk, vectors [][]float32) error
}

// Ingest cuts the documents into chunks, embeds them and saves the result.
// It returns how many chunks were indexed.
func Ingest(
	ctx context.Context,
	docs []domain.Document,
	chunker ingest.Chunker,
	embedder embed.Embedder,
	indexer Indexer,
	collection string,
) (int, error) {
	var chunks []domain.Chunk
	for _, doc := range docs {
		docChunks, err := chunker.Chunk(doc)
		if err != nil {
			return 0, fmt.Errorf("chunk %s: %w", doc.ID, err)
		}
		chunks = append(chunks, docChunks...)
	}
	if len(chunks) == 0 {
		return 0, errors.New("the documents produced no chunks")
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	vectors, err := embedder.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed chunks: %w", err)
	}

	if err := indexer.Replace(ctx, collection, chunks, vectors); err != nil {
		return 0, fmt.Errorf("save chunks: %w", err)
	}
	return len(chunks), nil
}
