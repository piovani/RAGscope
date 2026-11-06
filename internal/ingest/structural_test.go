package ingest

import (
	"strings"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

const lawText = `Lei de Exemplo
Dispõe sobre coisas.

Art. 1º Esta lei trata de coisas.

Art. 2º Para os fins desta lei, considera-se:
I - coisa: algo qualquer;
II - outra coisa: algo diferente, conforme o
art. 1º desta lei.

Art. 3-A Sem relação com os outros.
`

func TestStructuralOneChunkPerArticle(t *testing.T) {
	chunks, err := Structural{MaxWords: 100}.Chunk(domain.Document{ID: "lei", Title: "Lei", Text: lawText})
	if err != nil {
		t.Fatal(err)
	}

	// preamble + 3 articles. The "art. 1º" in the middle of a sentence is not a new article.
	if len(chunks) != 4 {
		t.Fatalf("got %d chunks, want 4: %+v", len(chunks), chunks)
	}

	wantArticles := []string{"", "1", "2", "3-A"}
	for i, want := range wantArticles {
		if got := chunks[i].Metadata["article"]; got != want {
			t.Errorf("chunk %d article = %q, want %q", i, got, want)
		}
	}

	if !strings.Contains(chunks[2].Text, "II - outra coisa") {
		t.Errorf("article 2 lost its items: %q", chunks[2].Text)
	}
	if strings.Contains(chunks[2].Text, "Sem relação") {
		t.Errorf("article 2 leaked into the next one: %q", chunks[2].Text)
	}
	if chunks[3].ID != "lei-0003" || chunks[3].Index != 3 {
		t.Errorf("last chunk id/index = %s/%d", chunks[3].ID, chunks[3].Index)
	}
}

func TestStructuralSplitsLongArticles(t *testing.T) {
	text := "Art. 1º " + strings.Repeat("palavra ", 25)

	chunks, err := Structural{MaxWords: 10}.Chunk(domain.Document{ID: "lei", Text: text})
	if err != nil {
		t.Fatal(err)
	}

	if len(chunks) < 3 {
		t.Fatalf("got %d chunks, want the long article split in several", len(chunks))
	}
	for _, c := range chunks {
		if c.Metadata["article"] != "1" {
			t.Errorf("chunk %s lost its article: %v", c.ID, c.Metadata)
		}
		if n := len(strings.Fields(c.Text)); n > 10 {
			t.Errorf("chunk %s has %d words, limit is 10", c.ID, n)
		}
	}
}

func TestStructuralWithoutArticlesFallsBackToWindows(t *testing.T) {
	chunks, err := Structural{MaxWords: 3}.Chunk(domain.Document{ID: "texto", Text: "a b c d e"})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Errorf("got %d chunks, want 2", len(chunks))
	}
}

func TestStructuralRejectsBadSettings(t *testing.T) {
	if _, err := (Structural{MaxWords: 0}).Chunk(domain.Document{Text: "x"}); err == nil {
		t.Error("expected an error for MaxWords = 0")
	}
}
