package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

// fakeServer scores each text by its length and answers in a shuffled order,
// like a real server may do, to prove we use the index and not the position.
func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		type item struct {
			Index int     `json:"index"`
			Score float64 `json:"score"`
		}
		var out []item
		for i := len(body.Texts) - 1; i >= 0; i-- { // reversed on purpose
			out = append(out, item{Index: i, Score: float64(len(body.Texts[i]))})
		}
		json.NewEncoder(w).Encode(out)
	}))
}

func candidate(id, text string) domain.ScoredChunk {
	return domain.ScoredChunk{Chunk: domain.Chunk{ID: id, Text: text}}
}

func TestRerankSortsByScoreAndKeepsTopN(t *testing.T) {
	srv := fakeServer(t)
	defer srv.Close()

	candidates := []domain.ScoredChunk{
		candidate("short", "aa"),
		candidate("longest", "aaaaaaaa"),
		candidate("middle", "aaaa"),
	}

	got, err := NewTEI(srv.URL).Rerank(context.Background(), "q", candidates, 2)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 || got[0].ID != "longest" || got[1].ID != "middle" {
		t.Fatalf("got %+v, want longest then middle", got)
	}
	if got[0].Score != 8 {
		t.Errorf("score = %v, want the reranker score (8)", got[0].Score)
	}
}

func TestRerankWithNoCandidates(t *testing.T) {
	got, err := NewTEI("http://unused.invalid").Rerank(context.Background(), "q", nil, 5)
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want nothing and no error", got, err)
	}
}
