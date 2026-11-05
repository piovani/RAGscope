// Package ingest turns raw sources into chunks.
//
// Planned Chunker implementations: fixed, recursive, structural (by article), semantic.
package ingest

import "github.com/piovani/RAGscope/internal/domain"

// Chunker splits a document into chunks.
type Chunker interface {
	Name() string
	Chunk(doc domain.Document) ([]domain.Chunk, error)
}
