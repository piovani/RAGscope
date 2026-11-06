package retrieve

import (
	"context"
	"errors"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

// ranked always answers with the same chunk IDs, in order.
type ranked struct {
	name string
	ids  []string
	err  error
}

func (r ranked) Name() string { return r.name }
func (r ranked) Retrieve(context.Context, string, int) ([]domain.ScoredChunk, error) {
	var out []domain.ScoredChunk
	for _, id := range r.ids {
		out = append(out, domain.ScoredChunk{Chunk: domain.Chunk{ID: id}})
	}
	return out, r.err
}

func ids(results []domain.ScoredChunk) []string {
	var out []string
	for _, r := range results {
		out = append(out, r.ID)
	}
	return out
}

func TestHybridRewardsChunksBothRetrieversLike(t *testing.T) {
	vector := ranked{name: "vector", ids: []string{"a", "b", "c"}}
	bm25 := ranked{name: "bm25", ids: []string{"d", "b", "a"}}

	got, err := Hybrid{Retrievers: []Retriever{vector, bm25}}.Retrieve(context.Background(), "q", 3)
	if err != nil {
		t.Fatal(err)
	}

	// "a" is 1st and 3rd, "b" is 2nd and 2nd: both are liked by both retrievers.
	// "a" wins by a hair, because 1/61 + 1/63 is a little more than 2/62 (the top
	// position counts more). After them comes "d", the best of the single votes.
	want := []string{"a", "b", "d"}
	if g := ids(got); len(g) != 3 || g[0] != want[0] || g[1] != want[1] || g[2] != want[2] {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestHybridStopsOnError(t *testing.T) {
	broken := ranked{name: "bm25", err: errors.New("boom")}

	_, err := Hybrid{Retrievers: []Retriever{ranked{name: "vector"}, broken}}.Retrieve(context.Background(), "q", 3)
	if err == nil {
		t.Fatal("expected the retriever error")
	}
}
