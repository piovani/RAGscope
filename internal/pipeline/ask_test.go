package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/generate"
)

type fakeRetriever struct {
	chunks []domain.ScoredChunk
	err    error
}

func (f fakeRetriever) Name() string { return "fake" }

func (f fakeRetriever) Retrieve(context.Context, string, int) ([]domain.ScoredChunk, error) {
	return f.chunks, f.err
}

type fakeGenerator struct {
	resp   generate.Response
	calls  int
	gotReq generate.Request
}

func (f *fakeGenerator) Name() string { return "fake" }

func (f *fakeGenerator) Generate(_ context.Context, req generate.Request) (generate.Response, error) {
	f.calls++
	f.gotReq = req
	return f.resp, nil
}

func scored(id string) domain.ScoredChunk {
	return domain.ScoredChunk{Chunk: domain.Chunk{ID: id}, Score: 0.5}
}

func TestAnswererNoSourcesRefusesWithoutModel(t *testing.T) {
	t.Parallel()

	gen := &fakeGenerator{}
	a := Answerer{Retriever: fakeRetriever{}, Generator: gen, K: 3}

	got, err := a.Ask(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Refused {
		t.Error("Refused = false, want true")
	}
	if gen.calls != 0 {
		t.Errorf("generator called %d times, want 0", gen.calls)
	}
}

func TestAnswererModelRefusalPassesThrough(t *testing.T) {
	t.Parallel()

	gen := &fakeGenerator{resp: generate.Response{Refused: true}}
	a := Answerer{Retriever: fakeRetriever{chunks: []domain.ScoredChunk{scored("a")}}, Generator: gen, K: 3}

	got, err := a.Ask(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Refused || len(got.Sources) != 1 {
		t.Errorf("got %+v, want refused with 1 source", got)
	}
}

func TestAnswererKeepsSourceOrder(t *testing.T) {
	t.Parallel()

	gen := &fakeGenerator{resp: generate.Response{Answer: "yes [1]", Usage: generate.Usage{InputTokens: 7}}}
	chunks := []domain.ScoredChunk{scored("b"), scored("a"), scored("c")}
	a := Answerer{Retriever: fakeRetriever{chunks: chunks}, Generator: gen, K: 3}

	got, err := a.Ask(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "yes [1]" || got.Usage.InputTokens != 7 {
		t.Errorf("got %+v", got)
	}
	for i, want := range []string{"b", "a", "c"} {
		if got.Sources[i].ID != want || gen.gotReq.Context[i].ID != want {
			t.Errorf("position %d: source %q, model saw %q, want %q", i, got.Sources[i].ID, gen.gotReq.Context[i].ID, want)
		}
	}
}

func TestAnswererRetrievalError(t *testing.T) {
	t.Parallel()

	gen := &fakeGenerator{}
	a := Answerer{Retriever: fakeRetriever{err: errors.New("boom")}, Generator: gen, K: 3}

	if _, err := a.Ask(context.Background(), "q"); err == nil {
		t.Fatal("want error")
	}
	if gen.calls != 0 {
		t.Error("generator must not be called")
	}
}
