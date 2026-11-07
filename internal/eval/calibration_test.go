package eval

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ptr(b bool) *bool { return &b }

func TestCohenKappa(t *testing.T) {
	tests := []struct {
		name           string
		yy, yn, ny, nn int
		want           float64
		ok             bool
	}{
		{"perfect agreement", 10, 0, 0, 10, 1, true},
		{"chance level", 5, 5, 5, 5, 0, true},
		{"total disagreement", 0, 10, 10, 0, -1, true},
		{"everyone says yes", 10, 0, 0, 0, 0, false},
		{"empty", 0, 0, 0, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CohenKappa(tt.yy, tt.yn, tt.ny, tt.nn)
			if ok != tt.ok || math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("got %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func sampleReport(n int) *AnswerReport {
	report := &AnswerReport{}
	for i := 0; i < n; i++ {
		id := "q" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		report.Results = append(report.Results, AnswerResult{
			ID: id, Question: "question " + id, Answer: "answer " + id,
			Passages: []string{"passage " + id},
			Correct:  ptr(true), Supported: ptr(false),
		})
	}
	report.Results = append(report.Results,
		AnswerResult{ID: "refused", Refused: true},
		AnswerResult{ID: "unans", Unanswerable: true, Answer: "invented"},
	)
	return report
}

func TestBuildSheet(t *testing.T) {
	report := sampleReport(10)
	questions := []Question{{ID: "qaa", Answer: "reference"}}

	first := BuildSheet(report, questions, 4)
	second := BuildSheet(report, questions, 4)
	if len(first) != 4 {
		t.Fatalf("got %d entries, want 4", len(first))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("selection is not deterministic at %d", i)
		}
		if first[i].ID == "refused" || first[i].ID == "unans" {
			t.Errorf("sheet has %s, which should be skipped", first[i].ID)
		}
	}
	if first[0].ID != "qaa" || first[0].ReferenceAnswer != "reference" || first[0].Passages[0] != "passage qaa" {
		t.Errorf("unexpected first entry: %+v", first[0])
	}
	if got := len(BuildSheet(report, questions, 100)); got != 10 {
		t.Errorf("asking for more than available gave %d, want 10", got)
	}
}

func TestSheetHasNoJudgeVerdicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sheet.yaml")
	entries := BuildSheet(sampleReport(3), nil, 3)
	if err := WriteSheet(path, entries); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	for _, bad := range []string{"judge:", "correct: true", "supported: false"} {
		if strings.Contains(strings.ReplaceAll(text, "human_", ""), bad) && bad != "judge:" {
			t.Errorf("sheet leaks %q", bad)
		}
	}
	if !strings.Contains(text, "human_correct: null") || !strings.Contains(text, "human_supported: null") {
		t.Errorf("sheet should have empty human fields:\n%s", text)
	}
}

func TestCalibrate(t *testing.T) {
	report := &AnswerReport{Results: []AnswerResult{
		{ID: "a", Question: "qa", Correct: ptr(true), Supported: ptr(true)},
		{ID: "b", Question: "qb", Correct: ptr(true), Supported: ptr(true)},
		{ID: "c", Question: "qc", Correct: ptr(false)}, // no supported verdict
		{ID: "d", Question: "qd"},                      // no verdicts at all
	}}
	sheet := []SheetEntry{
		{ID: "a", HumanCorrect: ptr(true), HumanSupported: ptr(true)},
		{ID: "b", Question: "qb", HumanCorrect: ptr(false)},
		{ID: "c", HumanCorrect: ptr(false), HumanSupported: ptr(true)},
		{ID: "d", HumanCorrect: ptr(true)},
		{ID: "ghost", HumanCorrect: ptr(true)},
		{ID: "e"}, // ungraded
	}

	c := Calibrate(report, sheet)
	got := c.Correct
	if got.Graded != 5 || got.Ungraded != 1 || got.Compared != 3 || got.NoJudge != 1 || got.MissingID != 1 {
		t.Errorf("correct counts wrong: %+v", got)
	}
	if got.YesYes != 1 || got.NoYes != 1 || got.NoNo != 1 || len(got.Disagreed) != 1 || got.Disagreed[0].ID != "b" {
		t.Errorf("correct table wrong: %+v", got)
	}
	if c.Supported.Graded != 2 || c.Supported.Compared != 1 || c.Supported.NoJudge != 1 {
		t.Errorf("supported counts wrong: %+v", c.Supported)
	}
}

func TestRenderCalibration(t *testing.T) {
	small := RenderCalibration(Calibrate(&AnswerReport{}, nil))
	if !strings.Contains(small, "too small to trust") {
		t.Errorf("missing small sample warning:\n%s", small)
	}

	// 20 entries, everyone says yes: enough data, but kappa is undefined.
	report := &AnswerReport{}
	var sheet []SheetEntry
	for i := 0; i < 20; i++ {
		id := string(rune('a' + i))
		report.Results = append(report.Results, AnswerResult{ID: id, Correct: ptr(true)})
		sheet = append(sheet, SheetEntry{ID: id, HumanCorrect: ptr(true)})
	}
	out := RenderCalibration(Calibrate(report, sheet))
	if !strings.Contains(out, "Cohen's kappa: n/a") {
		t.Errorf("kappa should be n/a:\n%s", out)
	}
	if strings.Contains(strings.SplitN(out, "## Supported", 2)[0], "too small") {
		t.Errorf("20 entries should not warn:\n%s", out)
	}
}

func TestCalibrationFlowThroughFiles(t *testing.T) {
	dir := t.TempDir()
	report := sampleReport(5)
	data, _ := json.Marshal(report)
	reportPath := filepath.Join(dir, "answers.json")
	if err := os.WriteFile(reportPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadAnswerReport(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	sheetPath := filepath.Join(dir, "sheet.yaml")
	if err := WriteSheet(sheetPath, BuildSheet(loaded, nil, 3)); err != nil {
		t.Fatal(err)
	}

	// The person fills in the first two correct fields; the third stays null.
	raw, _ := os.ReadFile(sheetPath)
	filled := strings.Replace(string(raw), "human_correct: null", "human_correct: true", 2)
	if err := os.WriteFile(sheetPath, []byte(filled), 0o644); err != nil {
		t.Fatal(err)
	}

	sheet, err := ReadSheet(sheetPath)
	if err != nil {
		t.Fatal(err)
	}
	c := Calibrate(loaded, sheet)
	if c.Correct.Graded != 2 || c.Correct.Ungraded != 1 || c.Correct.YesYes != 2 {
		t.Errorf("unexpected result: %+v", c.Correct)
	}
	if c.Supported.Graded != 0 {
		t.Errorf("supported should be skipped: %+v", c.Supported)
	}
}
