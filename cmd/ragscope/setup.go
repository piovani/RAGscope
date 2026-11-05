package main

import (
	"context"
	"fmt"
	"os"

	"github.com/piovani/RAGscope/internal/config"
	"github.com/piovani/RAGscope/internal/embed"
	"github.com/piovani/RAGscope/internal/ingest"
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
	if c.Strategy != "fixed" {
		return nil, fmt.Errorf("chunking strategy %q is not implemented yet (only \"fixed\")", c.Strategy)
	}
	return ingest.Fixed{Size: c.MaxTokens, Overlap: c.Overlap}, nil
}
