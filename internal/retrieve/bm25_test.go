package retrieve

import (
	"context"
	"reflect"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

func TestTokenize(t *testing.T) {
	got := tokenize("O Controlador, conforme o Art. 5º: INFORMAÇÃO de dados!")
	want := []string{"controlador", "conforme", "art", "5o", "informacao", "dados"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenize = %v, want %v", got, want)
	}
}

func testChunks() []domain.Chunk {
	return []domain.Chunk{
		{ID: "dado", Text: "dado pessoal é informação relacionada a pessoa natural"},
		{ID: "controlador", Text: "controlador é quem decide sobre o tratamento"},
		{ID: "operador", Text: "operador realiza o tratamento em nome do controlador"},
	}
}

func TestBM25FindsTheRightChunk(t *testing.T) {
	bm25 := NewBM25(testChunks())

	got, err := bm25.Retrieve(context.Background(), "O que é INFORMAÇÃO relacionada a pessoa?", 3)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) == 0 || got[0].ID != "dado" {
		t.Fatalf("results = %+v, want \"dado\" first", got)
	}
	// Chunks that share no word with the question are not returned.
	if len(got) != 1 {
		t.Errorf("got %d results, want only the matching chunk", len(got))
	}
}

func TestBM25PrefersTheRarerWord(t *testing.T) {
	// "tratamento" is in two chunks and "operador" in only one, so a question
	// with both should rank the "operador" chunk first.
	got, _ := NewBM25(testChunks()).Retrieve(context.Background(), "operador tratamento", 3)

	if len(got) < 2 || got[0].ID != "operador" {
		t.Fatalf("results = %+v, want \"operador\" first", got)
	}
}

func TestBM25LimitsToK(t *testing.T) {
	got, _ := NewBM25(testChunks()).Retrieve(context.Background(), "tratamento controlador", 1)
	if len(got) != 1 {
		t.Errorf("got %d results, want 1", len(got))
	}
}

func TestBM25WithoutChunks(t *testing.T) {
	got, err := NewBM25(nil).Retrieve(context.Background(), "qualquer coisa", 5)
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want nothing and no error", got, err)
	}
}
