package pipeline

import (
	"context"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/ingest"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Name() string { return "fake" }
func (fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

type fakeIndexer struct {
	collection string
	chunks     []domain.Chunk
}

func (f *fakeIndexer) Replace(_ context.Context, collection string, chunks []domain.Chunk, _ [][]float32) error {
	f.collection, f.chunks = collection, chunks
	return nil
}

func TestIngest(t *testing.T) {
	docs := []domain.Document{
		{ID: "a", Text: "one two three four"},
		{ID: "b", Text: "five six"},
	}
	indexer := &fakeIndexer{}

	n, err := Ingest(context.Background(), docs, ingest.Fixed{Size: 2}, fakeEmbedder{}, indexer, "baseline")
	if err != nil {
		t.Fatal(err)
	}

	// "a" gives 2 chunks, "b" gives 1.
	if n != 3 || len(indexer.chunks) != 3 {
		t.Errorf("indexed %d chunks (saved %d), want 3", n, len(indexer.chunks))
	}
	if indexer.collection != "baseline" {
		t.Errorf("collection = %q", indexer.collection)
	}
}

func TestIngestFailsWithoutChunks(t *testing.T) {
	docs := []domain.Document{{ID: "empty", Text: "   "}}

	_, err := Ingest(context.Background(), docs, ingest.Fixed{Size: 2}, fakeEmbedder{}, &fakeIndexer{}, "x")
	if err == nil {
		t.Fatal("expected an error when there is nothing to index")
	}
}
