package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeOllama answers /api/embed with one 2-d vector per input: [len(text), 1].
// It also records the model name and how many requests it got.
func fakeOllama(t *testing.T, model *string, requests *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++

		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.URL.Path != "/api/embed" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		*model = body.Model

		out := make([][]float32, len(body.Input))
		for i, text := range body.Input {
			out[i] = []float32{float32(len(text)), 1}
		}
		json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
}

func TestOllamaEmbedKeepsOrderAndSplitsBatches(t *testing.T) {
	var model string
	var requests int
	srv := fakeOllama(t, &model, &requests)
	defer srv.Close()

	// 70 texts of different lengths: 3 requests (32 + 32 + 6).
	texts := make([]string, 70)
	for i := range texts {
		texts[i] = strings.Repeat("a", i+1)
	}

	vectors, err := NewOllama(srv.URL, "bge-m3").Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}

	if requests != 3 {
		t.Errorf("requests = %d, want 3", requests)
	}
	if model != "bge-m3" {
		t.Errorf("model sent = %q, want bge-m3", model)
	}
	for i, v := range vectors {
		if v[0] != float32(i+1) {
			t.Fatalf("vector %d is out of order: %v", i, v)
		}
	}
}

func TestOllamaEmbedReportsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `model "nope" not found, try pulling it first`, http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := NewOllama(srv.URL, "nope").Embed(context.Background(), []string{"hello"})
	if err == nil || !strings.Contains(err.Error(), "try pulling it first") {
		t.Fatalf("error = %v, want it to include the server message", err)
	}
}
