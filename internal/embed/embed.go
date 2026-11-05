// Package embed turns text into vectors.
package embed

import "context"

// Embedder converts texts into dense vectors. The result has one vector per
// input text, in the same order.
type Embedder interface {
	Name() string
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
