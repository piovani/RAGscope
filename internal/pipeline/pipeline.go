// Package pipeline wires concrete stages together from an experiment config.
package pipeline

import (
	"github.com/piovani/RAGscope/internal/embed"
	"github.com/piovani/RAGscope/internal/generate"
	"github.com/piovani/RAGscope/internal/ingest"
	"github.com/piovani/RAGscope/internal/rerank"
	"github.com/piovani/RAGscope/internal/retrieve"
)

// Pipeline is one fully wired RAG configuration. Reranker may be nil.
type Pipeline struct {
	Chunker   ingest.Chunker
	Embedder  embed.Embedder
	Retriever retrieve.Retriever
	Reranker  rerank.Reranker
	Generator generate.Generator
}
