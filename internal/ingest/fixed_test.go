package ingest

import (
	"strings"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

func TestFixedChunk(t *testing.T) {
	doc := domain.Document{ID: "lei", Title: "Lei", Text: "a b c d e f g"}

	tests := []struct {
		name string
		size int
		over int
		want []string
	}{
		{"no overlap", 3, 0, []string{"a b c", "d e f", "g"}},
		{"with overlap", 3, 1, []string{"a b c", "c d e", "e f g"}},
		{"bigger than the text", 100, 10, []string{"a b c d e f g"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks, err := Fixed{Size: tt.size, Overlap: tt.over}.Chunk(doc)
			if err != nil {
				t.Fatal(err)
			}

			var got []string
			for _, c := range chunks {
				got = append(got, c.Text)
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("chunks = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFixedChunkSetsIDsAndPositions(t *testing.T) {
	doc := domain.Document{ID: "lei", Text: "a b c d"}

	chunks, err := Fixed{Size: 2}.Chunk(doc)
	if err != nil {
		t.Fatal(err)
	}

	if chunks[1].ID != "lei-0001" || chunks[1].Index != 1 || chunks[1].DocumentID != "lei" {
		t.Errorf("second chunk = %+v", chunks[1])
	}
}

func TestFixedChunkEmptyText(t *testing.T) {
	chunks, err := Fixed{Size: 5}.Chunk(domain.Document{ID: "vazio", Text: "  \n "})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 0 {
		t.Errorf("got %d chunks, want none", len(chunks))
	}
}

func TestFixedChunkRejectsBadSettings(t *testing.T) {
	doc := domain.Document{ID: "x", Text: "a b c"}

	for _, f := range []Fixed{{Size: 0}, {Size: 3, Overlap: 3}, {Size: 3, Overlap: -1}} {
		if _, err := f.Chunk(doc); err == nil {
			t.Errorf("Fixed%+v: expected an error", f)
		}
	}
}
