package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/piovani/RAGscope/internal/eval"
)

func TestBootstrapMeanOfEqualValuesHasNoSpread(t *testing.T) {
	mean, low, high := bootstrapMean([]float64{0.5, 0.5, 0.5, 0.5})

	if mean != 0.5 || low != 0.5 || high != 0.5 {
		t.Errorf("got mean=%v low=%v high=%v, want all 0.5", mean, low, high)
	}
}

func TestBootstrapMeanIntervalContainsTheMean(t *testing.T) {
	values := []float64{1, 0, 1, 1, 0, 0, 1, 0, 1, 1}

	mean, low, high := bootstrapMean(values)

	if mean != 0.6 {
		t.Errorf("mean = %v, want 0.6", mean)
	}
	if low > mean || high < mean || low >= high {
		t.Errorf("interval [%v, %v] should surround the mean %v", low, high, mean)
	}

	// Same data, same answer: the seed is fixed.
	_, low2, high2 := bootstrapMean(values)
	if low != low2 || high != high2 {
		t.Error("the interval changed between two runs on the same data")
	}
}

func TestBootstrapMeanWithNoValues(t *testing.T) {
	if mean, low, high := bootstrapMean(nil); mean != 0 || low != 0 || high != 0 {
		t.Errorf("got %v %v %v, want zeros", mean, low, high)
	}
}

// withResults gives a report one answer per question. ranks[i] is the position
// where question i found its evidence (0 = not found).
func withResults(r *eval.Report, ranks ...int) *eval.Report {
	for i, rank := range ranks {
		r.Results = append(r.Results, eval.QuestionResult{
			ID:    string(rune('a' + i)),
			Ranks: []int{rank},
		})
	}
	return r
}

func TestMarkdownMarksARealGain(t *testing.T) {
	// The baseline never finds anything; the other one always does.
	baseline := withResults(result("baseline", now, 5, 20, 0, 0), make([]int, 20)...)
	ones := make([]int, 20)
	for i := range ones {
		ones[i] = 1
	}
	better := withResults(result("better", now, 5, 20, 1, 1), ones...)

	var out bytes.Buffer
	Markdown(&out, []*eval.Report{baseline, better}, baseline, 0)

	text := out.String()
	for _, want := range []string{"+1.000 [+1.000, +1.000] *", "does not include 0"} {
		if !strings.Contains(text, want) {
			t.Errorf("output is missing %q:\n%s", want, text)
		}
	}
}

func TestMarkdownDoesNotMarkNoise(t *testing.T) {
	// Half of the questions get better and half get worse: nothing real.
	baseline := withResults(result("baseline", now, 5, 4, 0.5, 0.5), 1, 1, 0, 0)
	other := withResults(result("other", now, 5, 4, 0.5, 0.5), 0, 0, 1, 1)

	var out bytes.Buffer
	Markdown(&out, []*eval.Report{baseline, other}, baseline, 0)

	text := out.String()
	if strings.Contains(text, " *") {
		t.Errorf("a difference of pure noise should not be starred:\n%s", text)
	}
	if !strings.Contains(text, "+0.000 [-1.000, +1.000]") {
		t.Errorf("expected a wide interval around zero:\n%s", text)
	}
}

func TestWarningsForDifferentGoldenSets(t *testing.T) {
	a := result("a", now, 5, 20, 0.5, 0.4)
	b := result("b", now, 5, 20, 0.6, 0.5)
	old := result("old", now, 5, 20, 0.6, 0.5) // from before fingerprints existed

	a.GoldenSet, b.GoldenSet = "aaaaaaaaaaaa", "aaaaaaaaaaaa"
	if w := Warnings([]*eval.Report{a, b, old}); len(w) != 0 {
		t.Errorf("same golden set (and one unknown) should not warn: %v", w)
	}

	b.GoldenSet = "bbbbbbbbbbbb"
	w := Warnings([]*eval.Report{a, b})
	if len(w) != 1 || !strings.Contains(w[0], "versions of the golden set") {
		t.Errorf("warnings = %v, want one about the golden set", w)
	}
}

func TestScoresAtLooksOnlyAtTheFirstPositions(t *testing.T) {
	// Evidence found at positions 1, 3 and 0 (never).
	r := withResults(result("a", now, 5, 3, 0, 0), 1, 3, 0)

	top1, err := ScoresAt(r, 1)
	if err != nil {
		t.Fatal(err)
	}
	if top1.K != 1 || top1.Recall != 1.0/3 || top1.MRR != 1.0/3 {
		t.Errorf("top 1 = %+v, want recall and MRR of 1/3", top1)
	}

	top3, _ := ScoresAt(r, 3)
	if top3.Recall != 2.0/3 || top3.HitRate != 2.0/3 {
		t.Errorf("top 3 = %+v, want 2/3", top3)
	}
	if want := (1 + 1.0/3) / 3; top3.MRR != want {
		t.Errorf("top 3 MRR = %v, want %v", top3.MRR, want)
	}

	if _, err := ScoresAt(r, 10); err == nil {
		t.Error("asking for a top 10 when only 5 were retrieved should fail")
	}
}
