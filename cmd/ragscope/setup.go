package main

import (
	"context"
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

// allowPaid is set by the --allow-paid flag. Without it, no command builds a
// generator that bills per token, even if an API key is in the environment, so a
// stray key or a copied config can never spend money by accident.
var allowPaid bool

// newGenerator builds the model that writes the answers, as set in the experiment.
// RAGscope is not tied to one vendor: any provider below can be swapped in the
// YAML file without touching the rest of the pipeline.
func newGenerator(g config.Generator) (chatGenerator, error) {
	if g.NeedsPrices() && !allowPaid {
		return nil, fmt.Errorf("generator %s/%s is billed per token and may spend credits. Nothing was called. Add --allow-paid to run it on purpose", g.Provider, g.Model)
	}
	switch g.Provider {
	case "ollama":
		return generate.NewOllama(env("OLLAMA_URL", "http://localhost:11434"), g.Model, g.Temperature), nil
	case "openai":
		// Also reaches Azure OpenAI, GitHub Models, OpenRouter, vLLM, LM Studio...
		// A local server may have no key at all.
		key, err := apiKey(g, "OPENAI_API_KEY", g.NeedsPrices())
		if err != nil {
			return nil, err
		}
		return generate.NewOpenAI(g.BaseURL, key, g.Model, g.Temperature, g.PriceInputPerMTok, g.PriceOutputPerMTok), nil
	case "anthropic":
		key, err := apiKey(g, "ANTHROPIC_API_KEY", true)
		if err != nil {
			return nil, err
		}
		return generate.NewAnthropic(key, g.Model, g.Temperature, g.PriceInputPerMTok, g.PriceOutputPerMTok), nil
	}
	return nil, fmt.Errorf("generator provider %q is not implemented (use \"ollama\", \"openai\" or \"anthropic\")", g.Provider)
}

// apiKey reads the key from the environment, never from a file in the repository.
func apiKey(g config.Generator, defaultVar string, required bool) (string, error) {
	name := g.APIKeyEnv
	if name == "" {
		name = defaultVar
	}
	key := os.Getenv(name)
	if key == "" && required {
		return "", fmt.Errorf("%s is not set", name)
	}
	return key, nil
}
