package store

import (
	"context"
	"os"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

// This test needs a real Postgres with pgvector. Run `make up` and then:
//
//	TEST_DATABASE_URL=postgres://ragscope:ragscope@localhost:5432/ragscope?sslmode=disable go test ./internal/store
func TestReplaceAndSearch(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const collection = "store-test"
	chunks := []domain.Chunk{
		{ID: "a", DocumentID: "doc", Index: 0, Text: "pointing right"},
		{ID: "b", DocumentID: "doc", Index: 1, Text: "pointing up", Metadata: map[string]string{"title": "up"}},
	}
	vectors := [][]float32{{1, 0}, {0, 1}}

	if err := s.Replace(ctx, collection, chunks, vectors); err != nil {
		t.Fatal(err)
	}

	got, err := s.Search(ctx, collection, []float32{0.9, 0.1}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("results = %+v, want a then b", got)
	}
	if got[0].Score <= got[1].Score {
		t.Errorf("scores should be decreasing, got %v then %v", got[0].Score, got[1].Score)
	}
	if got[1].Metadata["title"] != "up" {
		t.Errorf("metadata was lost: %v", got[1].Metadata)
	}

	all, err := s.Chunks(ctx, collection)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != "a" || all[1].Text != "pointing up" {
		t.Errorf("Chunks = %+v, want a and b in document order", all)
	}

	// Replacing again must not keep the old rows.
	if err := s.Replace(ctx, collection, chunks[:1], vectors[:1]); err != nil {
		t.Fatal(err)
	}
	got, err = s.Search(ctx, collection, []float32{0, 1}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("after replace got %d rows, want 1", len(got))
	}
}
