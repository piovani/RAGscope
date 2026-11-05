package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeServer answers /embed with one 2-d vector per input: [len(text), 1].
func fakeServer(t *testing.T, requests *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++

		var body struct {
			Inputs []string `json:"inputs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		out := make([][]float32, len(body.Inputs))
		for i, text := range body.Inputs {
			out[i] = []float32{float32(len(text)), 1}
		}
		json.NewEncoder(w).Encode(out)
	}))
}

func TestEmbedKeepsOrderAndSplitsBatches(t *testing.T) {
	requests := 0
	srv := fakeServer(t, &requests)
	defer srv.Close()

	// 70 texts with different lengths: should take 3 requests (32 + 32 + 6).
	texts := make([]string, 70)
	for i := range texts {
		texts[i] = strings.Repeat("a", i+1)
	}

	vectors, err := NewTEI(srv.URL).Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}

	if requests != 3 {
		t.Errorf("requests = %d, want 3", requests)
	}
	if len(vectors) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(vectors), len(texts))
	}
	for i, v := range vectors {
		if v[0] != float32(i+1) {
			t.Fatalf("vector %d is out of order: %v", i, v)
		}
	}
}

func TestEmbedReportsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model is loading", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := NewTEI(srv.URL).Embed(context.Background(), []string{"hello"})
	if err == nil || !strings.Contains(err.Error(), "model is loading") {
		t.Fatalf("error = %v, want it to include the server message", err)
	}
}
