// Package domain holds the core types shared by every stage of the pipeline.
package domain

// Document is a source document before chunking.
type Document struct {
	ID     string
	Title  string
	Source string // URL or file path the document came from
	Text   string
}

// Chunk is a retrievable slice of a Document.
type Chunk struct {
	ID         string
	DocumentID string
	Index      int // position of the chunk inside its document
	Text       string
	Metadata   map[string]string // e.g. "article": "5", "law": "LGPD"
}

// ScoredChunk is a Chunk with a relevance score assigned by a retriever or reranker.
type ScoredChunk struct {
	Chunk
	Score float64
}
