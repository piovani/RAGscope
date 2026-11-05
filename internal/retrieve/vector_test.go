package retrieve

import (
	"context"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Name() string { return "fake" }
func (fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 2, 3}
	}
	return out, nil
}

// fakeSearcher remembers what it was asked, so the test can check it.
type fakeSearcher struct {
	collection string
	vector     []float32
	k          int
}

func (f *fakeSearcher) Search(_ context.Context, collection string, vector []float32, k int) ([]domain.ScoredChunk, error) {
	f.collection, f.vector, f.k = collection, vector, k
	return []domain.ScoredChunk{{Chunk: domain.Chunk{ID: "x"}, Score: 0.9}}, nil
}

func TestVectorRetrieve(t *testing.T) {
	searcher := &fakeSearcher{}
	r := Vector{Embedder: fakeEmbedder{}, Searcher: searcher, Collection: "baseline"}

	got, err := r.Retrieve(context.Background(), "what is personal data?", 4)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 || got[0].ID != "x" {
		t.Errorf("results = %+v", got)
	}
	if searcher.collection != "baseline" || searcher.k != 4 || len(searcher.vector) != 3 {
		t.Errorf("searcher got collection=%q k=%d vector=%v", searcher.collection, searcher.k, searcher.vector)
	}
}

func TestVectorRetrieveRejectsInvalidK(t *testing.T) {
	r := Vector{Embedder: fakeEmbedder{}, Searcher: &fakeSearcher{}}
	if _, err := r.Retrieve(context.Background(), "q", 0); err == nil {
		t.Fatal("expected an error for k = 0")
	}
}
