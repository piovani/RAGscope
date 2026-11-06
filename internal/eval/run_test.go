package eval

import (
	"context"
	"math"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

func chunk(id, text string) domain.ScoredChunk {
	return domain.ScoredChunk{Chunk: domain.Chunk{ID: id, Text: text}}
}

func TestFindRanks(t *testing.T) {
	retrieved := []domain.ScoredChunk{
		chunk("c1", "nothing useful here"),
		chunk("c2", "the controller decides about data"),
		chunk("c3", "Personal Data is any information"),
	}

	// Case is ignored, and a snippet that is not retrieved gets rank 0.
	got := findRanks(retrieved, []string{"personal data is", "the controller decides", "missing"})
	want := []int{3, 2, 0}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ranks = %v, want %v", got, want)
		}
	}
}

func TestQuestionResultMetrics(t *testing.T) {
	// Two evidence snippets: one found at position 2, the other not found.
	r := QuestionResult{Ranks: []int{2, 0}}

	if r.Hit(1) {
		t.Error("Hit(1) should be false: the first relevant chunk is at position 2")
	}
	if !r.Hit(2) {
		t.Error("Hit(2) should be true")
	}
	if got := r.Recall(5); got != 0.5 {
		t.Errorf("Recall(5) = %v, want 0.5", got)
	}
	if got := r.ReciprocalRank(); got != 0.5 {
		t.Errorf("ReciprocalRank = %v, want 0.5", got)
	}

	if got := (QuestionResult{Ranks: []int{0}}).ReciprocalRank(); got != 0 {
		t.Errorf("ReciprocalRank without a hit = %v, want 0", got)
	}
}

// fakeRetriever returns a fixed list per question text.
type fakeRetriever map[string][]domain.ScoredChunk

func (f fakeRetriever) Name() string { return "fake" }
func (f fakeRetriever) Retrieve(_ context.Context, query string, _ int) ([]domain.ScoredChunk, error) {
	return f[query], nil
}

func TestRunRetrieval(t *testing.T) {
	retriever := fakeRetriever{
		"first":  {chunk("a", "alpha evidence"), chunk("b", "other")}, // found at 1
		"second": {chunk("c", "other"), chunk("d", "beta evidence")},  // found at 2
		"third":  {chunk("e", "other"), chunk("f", "other")},          // not found
	}
	questions := []Question{
		{ID: "q1", Question: "first", Evidence: []string{"alpha evidence"}},
		{ID: "q2", Question: "second", Evidence: []string{"beta evidence"}},
		{ID: "q3", Question: "third", Evidence: []string{"gamma evidence"}},
		{ID: "q4", Question: "fourth", Unanswerable: true},
	}

	report, err := RunRetrieval(context.Background(), retriever, questions, 2)
	if err != nil {
		t.Fatal(err)
	}

	if report.Questions != 3 || report.Skipped != 1 {
		t.Errorf("questions=%d skipped=%d, want 3 and 1", report.Questions, report.Skipped)
	}

	// Cutoffs for k=2 are 1 and 2.
	if len(report.Cutoffs) != 2 || report.Cutoffs[0].K != 1 || report.Cutoffs[1].K != 2 {
		t.Fatalf("cutoffs = %+v", report.Cutoffs)
	}
	assertClose(t, "hit rate @1", report.Cutoffs[0].HitRate, 1.0/3)
	assertClose(t, "hit rate @2", report.Cutoffs[1].HitRate, 2.0/3)
	assertClose(t, "MRR", report.MRR, (1+0.5+0)/3)

	misses := report.Misses()
	if len(misses) != 1 || misses[0].ID != "q3" {
		t.Errorf("misses = %+v, want only q3", misses)
	}
}

func TestRunRetrievalNeedsAnswerableQuestions(t *testing.T) {
	questions := []Question{{ID: "q1", Question: "x", Unanswerable: true}}

	if _, err := RunRetrieval(context.Background(), fakeRetriever{}, questions, 5); err == nil {
		t.Fatal("expected an error when there is nothing to score")
	}
}

func TestCutoffs(t *testing.T) {
	tests := map[int][]int{
		1:  {1},
		5:  {1, 3, 5},
		7:  {1, 3, 5, 7},
		10: {1, 3, 5, 10},
		20: {1, 3, 5, 10, 20},
	}
	for k, want := range tests {
		got := cutoffs(k)
		if len(got) != len(want) {
			t.Errorf("cutoffs(%d) = %v, want %v", k, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("cutoffs(%d) = %v, want %v", k, got, want)
				break
			}
		}
	}
}

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}
