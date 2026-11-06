package embed

import (
	"context"
	"fmt"

	"github.com/piovani/RAGscope/internal/httpjson"
)

// Ollama embeds texts with a model served by a local Ollama. The model name
// is sent on every request, so the experiment config decides which one runs.
type Ollama struct {
	model  string
	client *httpjson.Client
}

func NewOllama(url, model string) *Ollama {
	return &Ollama{model: model, client: httpjson.New(url)}
}

func (o *Ollama) Name() string { return "ollama/" + o.model }

func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))

	// Small groups keep each request light, even for a big corpus.
	const batchSize = 32
	for start := 0; start < len(texts); start += batchSize {
		end := min(start+batchSize, len(texts))

		batch, err := o.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batch...)
	}

	return vectors, nil
}

func (o *Ollama) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// truncate: cut texts that are too long instead of failing the whole request.
	request := map[string]any{"model": o.model, "input": texts, "truncate": true}

	var answer struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := o.client.Post(ctx, "/api/embed", request, &answer); err != nil {
		return nil, err
	}
	if len(answer.Embeddings) != len(texts) {
		return nil, fmt.Errorf("sent %d texts but got %d vectors", len(texts), len(answer.Embeddings))
	}
	return answer.Embeddings, nil
}
