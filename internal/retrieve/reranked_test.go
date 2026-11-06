package retrieve

import (
	"context"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

// reverser is a reranker that just flips the order and cuts to topN.
type reverser struct{}

func (reverser) Name() string { return "reverser" }
func (reverser) Rerank(_ context.Context, _ string, c []domain.ScoredChunk, topN int) ([]domain.ScoredChunk, error) {
	out := make([]domain.ScoredChunk, 0, len(c))
	for i := len(c) - 1; i >= 0; i-- {
		out = append(out, c[i])
	}
	return out[:min(topN, len(out))], nil
}

func TestRerankedFetchesCandidatesThenCutsToK(t *testing.T) {
	base := ranked{name: "vector", ids: []string{"a", "b", "c", "d"}}
	r := Reranked{Base: base, Reranker: reverser{}, Candidates: 4}

	got, err := r.Retrieve(context.Background(), "q", 2)
	if err != nil {
		t.Fatal(err)
	}

	if g := ids(got); len(g) != 2 || g[0] != "d" || g[1] != "c" {
		t.Errorf("got %v, want [d c]", g)
	}
	if r.Name() != "vector+rerank" {
		t.Errorf("name = %q", r.Name())
	}
}
