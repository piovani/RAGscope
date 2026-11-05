// Package embed provides embedding clients (TEI sidecar, hosted providers).
package embed

import "context"

// Embedder converts texts into dense vectors. Output order matches input order.
type Embedder interface {
	Name() string
	Dimensions() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
