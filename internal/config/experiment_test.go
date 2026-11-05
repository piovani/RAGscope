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

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
