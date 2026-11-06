package main

import (
	"context"
	"fmt"
	"os"

	"github.com/piovani/RAGscope/internal/config"
	"github.com/piovani/RAGscope/internal/embed"
	"github.com/piovani/RAGscope/internal/ingest"
	"github.com/piovani/RAGscope/internal/rerank"
	"github.com/piovani/RAGscope/internal/retrieve"
	"github.com/piovani/RAGscope/internal/store"
)

// The defaults match deployments/docker-compose.yml and .env.example, so the
// commands work locally without any environment variables.
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func openStore(ctx context.Context) (*store.Store, error) {
	url := env("DATABASE_URL", "postgres://ragscope:ragscope@localhost:5432/ragscope?sslmode=disable")
	return store.Open(ctx, url)
}

func newEmbedder() embed.Embedder {
	return embed.NewTEI(env("TEI_EMBED_URL", "http://localhost:8081"))
}

func newChunker(c config.Chunking) (ingest.Chunker, error) {
	switch c.Strategy {
	case "fixed":
		return ingest.Fixed{Size: c.MaxTokens, Overlap: c.Overlap}, nil
	case "structural":
		return ingest.Structural{MaxWords: c.MaxTokens, Overlap: c.Overlap}, nil
	}
	return nil, fmt.Errorf("chunking strategy %q is not implemented yet (use \"fixed\" or \"structural\")", c.Strategy)
}

// newRetriever builds the retrieval described by the experiment: the search
// mode, and the reranker on top of it when there is one.
func newRetriever(ctx context.Context, exp *config.Experiment, db *store.Store) (retrieve.Retriever, error) {
	collection := exp.Collection()
	vector := retrieve.Vector{Embedder: newEmbedder(), Searcher: db, Collection: collection}

	var retriever retrieve.Retriever
	switch exp.Retrieval.Mode {
	case "vector":
		retriever = vector
	case "bm25":
		bm25, err := newBM25(ctx, db, collection)
		if err != nil {
			return nil, err
		}
		retriever = bm25
	case "hybrid":
		if exp.Retrieval.Fusion != "rrf" {
			return nil, fmt.Errorf("fusion %q is not implemented yet (only \"rrf\")", exp.Retrieval.Fusion)
		}
		bm25, err := newBM25(ctx, db, collection)
		if err != nil {
			return nil, err
		}
		retriever = retrieve.Hybrid{Retrievers: []retrieve.Retriever{vector, bm25}}
	default:
		return nil, fmt.Errorf("retrieval mode %q is not implemented yet", exp.Retrieval.Mode)
	}

	if exp.Reranker != nil {
		reranker := rerank.NewTEI(env("TEI_RERANK_URL", "http://localhost:8082"))
		retriever = retrieve.Reranked{Base: retriever, Reranker: reranker, Candidates: exp.Retrieval.K}
	}
	return retriever, nil
}

// newBM25 loads the collection's chunks to build the in-memory index.
func newBM25(ctx context.Context, db *store.Store, collection string) (*retrieve.BM25, error) {
	chunks, err := db.Chunks(ctx, collection)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("collection %q is empty. Did you run ingest?", collection)
	}
	return retrieve.NewBM25(chunks), nil
}
