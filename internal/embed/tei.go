package embed

import (
	"context"
	"fmt"

	"github.com/piovani/RAGscope/internal/tei"
)

// TEI gets embeddings from a Hugging Face Text Embeddings Inference server.
type TEI struct {
	client *tei.Client
}

func NewTEI(url string) *TEI {
	return &TEI{client: tei.New(url)}
}

func (t *TEI) Name() string { return "tei" }

func (t *TEI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))

	for start := 0; start < len(texts); start += tei.BatchSize {
		end := min(start+tei.BatchSize, len(texts))

		batch, err := t.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batch...)
	}

	return vectors, nil
}

func (t *TEI) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// truncate: cut texts that are too long instead of failing the whole request.
	request := map[string]any{"inputs": texts, "truncate": true}

	var vectors [][]float32
	if err := t.client.Post(ctx, "/embed", request, &vectors); err != nil {
		return nil, err
	}
	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("sent %d texts but got %d vectors", len(texts), len(vectors))
	}
	return vectors, nil
}
