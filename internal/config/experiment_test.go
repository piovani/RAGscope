package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `
name: baseline
corpus: br-legislation-v1
chunking: {strategy: fixed, max_tokens: 512}
embedding: {model: bge-m3}
retrieval: {mode: hybrid, k: 10, fusion: rrf}
reranker: {model: bge-reranker-v2-m3, top_n: 5}
generator: {provider: ollama, model: llama3, temperature: 0}
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "exp.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		wantErr string // substring; empty means success
	}{
		{name: "valid", yaml: validYAML},
		{
			name:    "unknown key is rejected",
			yaml:    validYAML + "\nchunkng: {}\n",
			wantErr: "chunkng",
		},
		{
			name:    "bad chunking strategy",
			yaml:    strings.Replace(validYAML, "strategy: fixed", "strategy: magic", 1),
			wantErr: "chunking.strategy",
		},
		{
			name:    "hybrid without fusion",
			yaml:    strings.Replace(validYAML, ", fusion: rrf", "", 1),
			wantErr: "fusion is required",
		},
		{
			name:    "top_n larger than k",
			yaml:    strings.Replace(validYAML, "top_n: 5", "top_n: 50", 1),
			wantErr: "top_n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			exp, err := Load(writeTemp(t, tt.yaml))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if exp.Name != "baseline" {
					t.Errorf("Name = %q, want baseline", exp.Name)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestCollectionIgnoresRetrievalSettings(t *testing.T) {
	a, err := Load(writeTemp(t, validYAML))
	if err != nil {
		t.Fatal(err)
	}

	// Same index, different retrieval: the two experiments must share a collection.
	b, err := Load(writeTemp(t, strings.Replace(validYAML, "mode: hybrid", "mode: bm25", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if a.Collection() != b.Collection() {
		t.Errorf("collections differ: %q and %q", a.Collection(), b.Collection())
	}

	// Different chunking: it needs its own index.
	c, err := Load(writeTemp(t, strings.Replace(validYAML, "strategy: fixed", "strategy: structural", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if a.Collection() == c.Collection() {
		t.Errorf("different chunking should not share the collection %q", a.Collection())
	}
}

func TestFinalK(t *testing.T) {
	withReranker, err := Load(writeTemp(t, validYAML))
	if err != nil {
		t.Fatal(err)
	}
	if got := withReranker.FinalK(); got != 5 {
		t.Errorf("FinalK with reranker = %d, want its top_n (5)", got)
	}

	withReranker.Reranker = nil
	if got := withReranker.FinalK(); got != 10 {
		t.Errorf("FinalK without reranker = %d, want retrieval k (10)", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRewriteAndProviderValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{"hyde is accepted", strings.Replace(validYAML, "fusion: rrf}", "fusion: rrf, rewrite: hyde}", 1), ""},
		{"unknown rewrite", strings.Replace(validYAML, "fusion: rrf}", "fusion: rrf, rewrite: magic}", 1), "retrieval.rewrite"},
		{"unknown provider", strings.Replace(validYAML, "provider: ollama", "provider: skynet", 1), "generator.provider"},
		{
			name:    "anthropic needs prices",
			yaml:    strings.Replace(validYAML, "provider: ollama, model: llama3", "provider: anthropic, model: some-model", 1),
			wantErr: "price_input_per_mtok",
		},
		{
			name: "anthropic with prices",
			yaml: strings.Replace(validYAML, "provider: ollama, model: llama3",
				"provider: anthropic, model: some-model, price_input_per_mtok: 1, price_output_per_mtok: 5", 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(writeTemp(t, tt.yaml))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestRewriteQueryCountDefault(t *testing.T) {
	t.Parallel()
	e := &Experiment{}
	if e.RewriteQueryCount() != 3 {
		t.Errorf("default = %d, want 3", e.RewriteQueryCount())
	}
	e.Retrieval.RewriteQueries = 5
	if e.RewriteQueryCount() != 5 {
		t.Errorf("explicit = %d, want 5", e.RewriteQueryCount())
	}
}
