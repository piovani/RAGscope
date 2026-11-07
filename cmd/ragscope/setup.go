package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/piovani/RAGscope/internal/config"
	"github.com/piovani/RAGscope/internal/embed"
	"github.com/piovani/RAGscope/internal/generate"
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

// newEmbedder uses the model named in the experiment, served by Ollama.
func newEmbedder(model string) embed.Embedder {
	return embed.NewOllama(env("OLLAMA_URL", "http://localhost:11434"), model)
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
	vector := retrieve.Vector{Embedder: newEmbedder(exp.Embedding.Model), Searcher: db, Collection: collection}

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

	retriever, err := withRewrite(retriever, exp)
	if err != nil {
		return nil, err
	}

	if exp.Reranker != nil {
		reranker := rerank.NewTEI(env("TEI_RERANK_URL", "http://localhost:8082"))
		retriever = retrieve.Reranked{Base: retriever, Reranker: reranker, Candidates: exp.Retrieval.K}
	}
	return retriever, nil
}

// withRewrite wraps the retriever with the query rewriting the experiment asks
// for. The experiment's own model does the rewriting, so a run is not mixed with
// a second model. Its calls are not counted in the cost of the answers.
func withRewrite(base retrieve.Retriever, exp *config.Experiment) (retrieve.Retriever, error) {
	switch exp.Retrieval.Rewrite {
	case "", "none":
		return base, nil
	case "hyde", "multi_query":
		llm, err := newGenerator(exp.Generator)
		if err != nil {
			return nil, err
		}
		if exp.Retrieval.Rewrite == "hyde" {
			return retrieve.HyDE{Base: base, LLM: llm}, nil
		}
		return retrieve.MultiQuery{Base: base, LLM: llm, N: exp.RewriteQueryCount()}, nil
	}
	return nil, fmt.Errorf("query rewriting %q is not implemented", exp.Retrieval.Rewrite)
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

// chatGenerator is a model that writes answers and can also take free-form
// prompts, which the judge and the query rewriters need.
type chatGenerator interface {
	generate.Generator
	Chat(ctx context.Context, system, user string) (generate.Response, error)
}

// newGenerator builds the model that writes the answers, as set in the experiment.
func newGenerator(g config.Generator) (chatGenerator, error) {
	switch g.Provider {
	case "ollama":
		return generate.NewOllama(env("OLLAMA_URL", "http://localhost:11434"), g.Model, g.Temperature), nil
	case "anthropic":
		// The key comes from the environment, never from a file in the repository.
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, errors.New("ANTHROPIC_API_KEY is not set")
		}
		return generate.NewAnthropic(key, g.Model, g.Temperature, g.PriceInputPerMTok, g.PriceOutputPerMTok), nil
	}
	return nil, fmt.Errorf("generator provider %q is not implemented yet (use \"ollama\" or \"anthropic\")", g.Provider)
}
