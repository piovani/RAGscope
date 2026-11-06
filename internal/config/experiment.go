// Package config loads and validates experiment definitions.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Experiment describes one fully specified RAG configuration.
type Experiment struct {
	Name      string    `yaml:"name"`
	Corpus    string    `yaml:"corpus"`
	Chunking  Chunking  `yaml:"chunking"`
	Embedding Embedding `yaml:"embedding"`
	Retrieval Retrieval `yaml:"retrieval"`
	Reranker  *Reranker `yaml:"reranker,omitempty"` // nil disables reranking
	Generator Generator `yaml:"generator"`
}

type Chunking struct {
	Strategy  string `yaml:"strategy"` // fixed | recursive | structural | semantic
	MaxTokens int    `yaml:"max_tokens"`
	Overlap   int    `yaml:"overlap,omitempty"`
}

type Embedding struct {
	Model string `yaml:"model"`
}

type Retrieval struct {
	Mode   string `yaml:"mode"` // vector | bm25 | hybrid
	K      int    `yaml:"k"`
	Fusion string `yaml:"fusion,omitempty"` // rrf (hybrid only)
}

type Reranker struct {
	Model string `yaml:"model"`
	TopN  int    `yaml:"top_n"`
}

type Generator struct {
	Provider    string  `yaml:"provider"` // anthropic | openai | ollama
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
}

// Collection names where the experiment's chunks are stored. It depends only on
// what changes the index (corpus, chunking and embedding model), not on the
// experiment name. So experiments that differ only in retrieval, reranking or
// generation share one index and need a single ingest.
func (e *Experiment) Collection() string {
	return fmt.Sprintf("%s_%s-%d-%d_%s",
		e.Corpus, e.Chunking.Strategy, e.Chunking.MaxTokens, e.Chunking.Overlap, e.Embedding.Model)
}

// FinalK is how many chunks the experiment hands to the generator: what is left
// after reranking, or the retrieval k when there is no reranker.
func (e *Experiment) FinalK() int {
	if e.Reranker != nil {
		return e.Reranker.TopN
	}
	return e.Retrieval.K
}

var (
	chunkStrategies = map[string]bool{"fixed": true, "recursive": true, "structural": true, "semantic": true}
	retrievalModes  = map[string]bool{"vector": true, "bm25": true, "hybrid": true}
)

// Load reads and validates an experiment YAML file.
func Load(path string) (*Experiment, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read experiment config: %w", err)
	}

	var exp Experiment
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true) // reject typos in config keys
	if err := dec.Decode(&exp); err != nil {
		return nil, fmt.Errorf("parse experiment config %s: %w", path, err)
	}
	if err := exp.Validate(); err != nil {
		return nil, fmt.Errorf("invalid experiment config %s: %w", path, err)
	}
	return &exp, nil
}

// Validate checks that the experiment is internally consistent.
func (e *Experiment) Validate() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if e.Name == "" {
		add("name is required")
	}
	if e.Corpus == "" {
		add("corpus is required")
	}
	if !chunkStrategies[e.Chunking.Strategy] {
		add("chunking.strategy %q is not one of fixed|recursive|structural|semantic", e.Chunking.Strategy)
	}
	if e.Chunking.MaxTokens <= 0 {
		add("chunking.max_tokens must be > 0")
	}
	if e.Embedding.Model == "" {
		add("embedding.model is required")
	}
	if !retrievalModes[e.Retrieval.Mode] {
		add("retrieval.mode %q is not one of vector|bm25|hybrid", e.Retrieval.Mode)
	}
	if e.Retrieval.K <= 0 {
		add("retrieval.k must be > 0")
	}
	if e.Retrieval.Mode == "hybrid" && e.Retrieval.Fusion == "" {
		add("retrieval.fusion is required when mode is hybrid")
	}
	if e.Reranker != nil {
		if e.Reranker.Model == "" {
			add("reranker.model is required when reranker is set")
		}
		if e.Reranker.TopN <= 0 || e.Reranker.TopN > e.Retrieval.K {
			add("reranker.top_n must be in (0, retrieval.k]")
		}
	}
	if e.Generator.Provider == "" || e.Generator.Model == "" {
		add("generator.provider and generator.model are required")
	}
	return errors.Join(errs...)
}
