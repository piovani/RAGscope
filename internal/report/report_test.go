package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/piovani/RAGscope/internal/eval"
)

// result builds a report with one cutoff, the one at its own k.
func result(name string, runAt time.Time, k, questions int, recall, mrr float64) *eval.Report {
	return &eval.Report{
		Experiment: name,
		Retriever:  "vector",
		K:          k,
		RunAt:      runAt,
		Questions:  questions,
		Cutoffs:    []eval.CutoffScore{{K: k, HitRate: recall, Recall: recall}},
		MRR:        mrr,
		LatencyP95: 120,
	}
}

var now = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

func TestLoadReadsSavedReports(t *testing.T) {
	dir := t.TempDir()
	for _, r := range []*eval.Report{
		result("baseline", now, 5, 20, 0.5, 0.4),
		result("bm25", now, 5, 20, 0.6, 0.5),
	} {
		if _, err := r.Save(dir); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("loaded %d reports, want 2", len(got))
	}
}

func TestLoadWithoutResults(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "ragscope eval") {
		t.Errorf("error = %v, want a hint to run eval", err)
	}
}

func TestLatestKeepsTheNewestRunOfEachExperiment(t *testing.T) {
	reports := []*eval.Report{
		result("baseline", now, 5, 20, 0.30, 0.3),
		result("baseline", now.Add(time.Hour), 5, 20, 0.50, 0.4), // newer
		result("bm25", now, 5, 20, 0.60, 0.5),
	}

	got := Latest(reports)

	if len(got) != 2 || got[0].Experiment != "baseline" || got[1].Experiment != "bm25" {
		t.Fatalf("got %d reports: %+v", len(got), got)
	}
	if got[0].Final().Recall != 0.50 {
		t.Errorf("baseline recall = %v, want the newer run (0.50)", got[0].Final().Recall)
	}
}

func TestMarkdownRanksAndComparesToBaseline(t *testing.T) {
	baseline := result("baseline", now, 5, 20, 0.50, 0.40)
	better := result("hybrid", now, 5, 20, 0.80, 0.70)
	reports := []*eval.Report{baseline, better}

	var out bytes.Buffer
	Markdown(&out, reports, baseline)
	text := out.String()

	// The best experiment comes first.
	if strings.Index(text, "| hybrid |") > strings.Index(text, "| baseline |") {
		t.Errorf("hybrid should be listed before baseline:\n%s", text)
	}
	for _, want := range []string{"+0.300", "+0.300", "baseline | baseline"} {
		if !strings.Contains(text, want) {
			t.Errorf("output is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Warning") {
		t.Errorf("unexpected warning:\n%s", text)
	}
}

func TestMarkdownWithoutBaselineHasNoDeltaColumns(t *testing.T) {
	var out bytes.Buffer
	Markdown(&out, []*eval.Report{result("a", now, 5, 20, 0.5, 0.4)}, nil)

	if strings.Contains(out.String(), "Δ") {
		t.Errorf("unexpected delta columns:\n%s", out.String())
	}
}

func TestWarnings(t *testing.T) {
	same := []*eval.Report{result("a", now, 5, 20, 0.5, 0.4), result("b", now, 5, 20, 0.6, 0.5)}
	if w := Warnings(same); len(w) != 0 {
		t.Errorf("warnings = %v, want none", w)
	}

	mixed := []*eval.Report{result("a", now, 5, 20, 0.5, 0.4), result("b", now, 10, 25, 0.6, 0.5)}
	if w := Warnings(mixed); len(w) != 2 {
		t.Errorf("warnings = %v, want one about k and one about questions", w)
	}
}
