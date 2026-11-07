package retrieve

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/generate"
)

// fakeChatter answers with a fixed text and records the prompts it received.
type fakeChatter struct {
	text    string
	err     error
	systems []string
	users   []string
}

func (f *fakeChatter) Chat(_ context.Context, system, user string) (generate.Response, error) {
	f.systems = append(f.systems, system)
	f.users = append(f.users, user)
	return generate.Response{Answer: f.text}, f.err
}

// fakeBase returns a fixed ranking per query (or a default) and records the
// queries and the k it received.
type fakeBase struct {
	byQuery  map[string][]string
	fallback []string
	queries  []string
	ks       []int
}

func (f *fakeBase) Name() string { return "base" }
func (f *fakeBase) Retrieve(_ context.Context, query string, k int) ([]domain.ScoredChunk, error) {
	f.queries = append(f.queries, query)
	f.ks = append(f.ks, k)
	list, ok := f.byQuery[query]
	if !ok {
		list = f.fallback
	}
	return ranked{ids: list}.Retrieve(context.Background(), query, k)
}

func TestHyDESearchesWithQuestionAndPassage(t *testing.T) {
	llm := &fakeChatter{text: "  Art. 7º O tratamento depende de consentimento.\n"}
	base := &fakeBase{fallback: []string{"a"}}

	got, err := HyDE{Base: base, LLM: llm}.Retrieve(context.Background(), "Quando posso tratar dados?", 5)
	if err != nil {
		t.Fatal(err)
	}

	want := "Quando posso tratar dados?\nArt. 7º O tratamento depende de consentimento."
	if len(base.queries) != 1 || base.queries[0] != want {
		t.Errorf("base queries = %q, want [%q]", base.queries, want)
	}
	if llm.users[0] != "Quando posso tratar dados?" {
		t.Errorf("LLM got user %q", llm.users[0])
	}
	if base.ks[0] != 5 {
		t.Errorf("base k = %d, want 5", base.ks[0])
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("got %v", ids(got))
	}
}

func TestHyDEErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		llm  *fakeChatter
	}{
		{"llm error", &fakeChatter{err: boom}},
		{"empty answer", &fakeChatter{text: "  \n "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := &fakeBase{}
			_, err := HyDE{Base: base, LLM: tt.llm}.Retrieve(context.Background(), "q", 3)
			if err == nil {
				t.Fatal("expected an error")
			}
			if len(base.queries) != 0 {
				t.Errorf("base must not be called, got %q", base.queries)
			}
		})
	}
	if _, err := (HyDE{Base: &fakeBase{}, LLM: &fakeChatter{err: boom}}).Retrieve(context.Background(), "q", 3); !errors.Is(err, boom) {
		t.Errorf("error should wrap the cause, got %v", err)
	}
}

func TestRewriteNames(t *testing.T) {
	base := &fakeBase{}
	if got := (HyDE{Base: base}).Name(); got != "base+hyde" {
		t.Errorf("hyde name = %q", got)
	}
	if got := (MultiQuery{Base: base}).Name(); got != "base+multiquery" {
		t.Errorf("multiquery name = %q", got)
	}
}

func TestParseRephrasings(t *testing.T) {
	tests := []struct {
		name string
		text string
		n    int
		want []string
	}{
		{"bullets", "- um\n* dois\n• três", 5, []string{"um", "dois", "três"}},
		{"numbering", "1. um\n2) dois\n10. três", 5, []string{"um", "dois", "três"}},
		{"blank lines and spaces", "\n  um  \n\n   \ndois\n", 5, []string{"um", "dois"}},
		{"duplicates", "um\nUM\ndois\num", 5, []string{"um", "dois"}},
		{"original repeated", "Qual é a Pergunta?\num", 5, []string{"um"}},
		{"cap at n", "um\ndois\ntrês\nquatro", 2, []string{"um", "dois"}},
		{"number inside text is kept", "2 anos é o prazo?", 5, []string{"2 anos é o prazo?"}},
		{"nothing usable", " \n-\n", 3, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRephrasings(tt.text, "qual é a pergunta?", tt.n)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMultiQueryFusesAllQueries(t *testing.T) {
	llm := &fakeChatter{text: "1. outra\n2. mais uma"}
	base := &fakeBase{byQuery: map[string][]string{
		"orig":     {"x", "a"},
		"outra":    {"y", "a"},
		"mais uma": {"z", "a"},
	}}

	got, err := MultiQuery{Base: base, LLM: llm, N: 2}.Retrieve(context.Background(), "orig", 2)
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"orig", "outra", "mais uma"}; !slices.Equal(base.queries, want) {
		t.Errorf("queries = %q, want %q", base.queries, want)
	}
	for _, k := range base.ks {
		if k != 20 {
			t.Errorf("pool = %d, want 20", k)
		}
	}
	// "a" is found by all three queries, so it beats x, y and z (found once).
	// Among the singles the tie goes to the smallest ID.
	if want := []string{"a", "x"}; !slices.Equal(ids(got), want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestMultiQueryPoolGrowsWithK(t *testing.T) {
	base := &fakeBase{}
	_, err := MultiQuery{Base: base, LLM: &fakeChatter{}}.Retrieve(context.Background(), "q", 15)
	if err != nil {
		t.Fatal(err)
	}
	if base.ks[0] != 30 {
		t.Errorf("pool = %d, want 30", base.ks[0])
	}
}

func TestMultiQueryWithoutUsableRephrasings(t *testing.T) {
	llm := &fakeChatter{text: "Q\n  \n- q"} // only the original, repeated
	base := &fakeBase{fallback: []string{"a"}}

	got, err := MultiQuery{Base: base, LLM: llm, N: 3}.Retrieve(context.Background(), "q", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(base.queries, []string{"q"}) {
		t.Errorf("queries = %q, want only the original", base.queries)
	}
	if !slices.Equal(ids(got), []string{"a"}) {
		t.Errorf("got %v", ids(got))
	}
}

func TestMultiQueryFewerThanN(t *testing.T) {
	base := &fakeBase{}
	_, err := MultiQuery{Base: base, LLM: &fakeChatter{text: "só uma"}, N: 4}.Retrieve(context.Background(), "q", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(base.queries, []string{"q", "só uma"}) {
		t.Errorf("queries = %q", base.queries)
	}
}

func TestMultiQueryLLMError(t *testing.T) {
	base := &fakeBase{}
	_, err := MultiQuery{Base: base, LLM: &fakeChatter{err: errors.New("boom")}}.Retrieve(context.Background(), "q", 3)
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(base.queries) != 0 {
		t.Errorf("base must not be called, got %q", base.queries)
	}
}

func TestMultiQueryDefaultsToThree(t *testing.T) {
	for _, n := range []int{0, -2} {
		llm := &fakeChatter{text: "a\nb\nc\nd\ne"}
		base := &fakeBase{}
		_, err := MultiQuery{Base: base, LLM: llm, N: n}.Retrieve(context.Background(), "q", 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(base.queries) != 4 { // original + 3
			t.Errorf("N=%d: %d queries, want 4", n, len(base.queries))
		}
	}
}
