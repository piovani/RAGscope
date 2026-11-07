package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// minCalibrationSample is the smallest number of compared entries we trust.
// Below it, one disagreement moves the numbers too much.
const minCalibrationSample = 20

// sheetHeader explains to the person how to fill the sheet.
const sheetHeader = `# Grading sheet for the answer judge.
#
# For every entry, read the question, the reference answer and the model answer.
#
# human_correct (required): true if the model answer says the same as the
#   reference answer, false otherwise.
# human_supported (optional, takes longer): true if EVERY claim in the model
#   answer is supported by the passages, false otherwise. Leave null to skip.
#
# The judge's verdicts are not shown here on purpose, so they do not
# influence your grades.
`

// SheetEntry is one answer for a person to grade.
type SheetEntry struct {
	ID              string   `yaml:"id"`
	Question        string   `yaml:"question"`
	ReferenceAnswer string   `yaml:"reference_answer"`
	ModelAnswer     string   `yaml:"model_answer"`
	Passages        []string `yaml:"passages"`
	HumanCorrect    *bool    `yaml:"human_correct"`
	HumanSupported  *bool    `yaml:"human_supported"`
}

// LoadAnswerReport reads an answer report saved by `ragscope eval-answers`.
func LoadAnswerReport(path string) (*AnswerReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read answer report: %w", err)
	}
	var report AnswerReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, fmt.Errorf("parse answer report %s: %w", path, err)
	}
	return &report, nil
}

// BuildSheet picks n real answers to answerable questions, evenly spaced over
// the report, so the same report always gives the same sheet. Refusals and
// unanswerable questions are left out: they are graded automatically.
func BuildSheet(report *AnswerReport, questions []Question, n int) []SheetEntry {
	reference := map[string]string{}
	for _, q := range questions {
		reference[q.ID] = q.Answer
	}

	var candidates []AnswerResult
	for _, res := range report.Results {
		if !res.Unanswerable && !res.Refused {
			candidates = append(candidates, res)
		}
	}

	var entries []SheetEntry
	for _, i := range evenlySpaced(len(candidates), n) {
		res := candidates[i]
		entries = append(entries, SheetEntry{
			ID:              res.ID,
			Question:        res.Question,
			ReferenceAnswer: reference[res.ID],
			ModelAnswer:     res.Answer,
			Passages:        res.Passages,
		})
	}
	return entries
}

// evenlySpaced returns n indexes spread over a list of the given size.
func evenlySpaced(size, n int) []int {
	if n <= 0 || size == 0 {
		return nil
	}
	if n >= size {
		n = size
	}
	indexes := make([]int, n)
	for i := range indexes {
		indexes[i] = i * size / n
	}
	return indexes
}

// WriteSheet saves the entries as YAML, with the instructions on top.
func WriteSheet(path string, entries []SheetEntry) error {
	body, err := yaml.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(sheetHeader+"\n"), body...), 0o644)
}

// ReadSheet reads a sheet filled in by a person.
func ReadSheet(path string) ([]SheetEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sheet: %w", err)
	}
	var entries []SheetEntry
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true) // catch typos like "human_corect"
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("parse sheet %s: %w", path, err)
	}
	return entries, nil
}

// Disagreement is an entry where the person and the judge said different things.
type Disagreement struct {
	ID       string
	Question string
}

// Agreement compares the judge with the person on one yes/no question.
// The counts are named human first, judge second.
type Agreement struct {
	Metric string

	Graded     int // entries the person graded (field not null)
	NoJudge    int // graded by the person, but the judge has no verdict
	MissingID  int // graded entries whose id is not in the answer report
	Compared   int
	YesYes     int
	YesNo      int // person yes, judge no
	NoYes      int // person no, judge yes
	NoNo       int
	Disagreed  []Disagreement
	Ungraded   int // entries with the field left null
	TotalSheet int
}

// The methods below return ok=false when the denominator is zero.

// Percent is the share of entries where both said the same.
func (a Agreement) Percent() (float64, bool) {
	if a.Compared == 0 {
		return 0, false
	}
	return 100 * float64(a.YesYes+a.NoNo) / float64(a.Compared), true
}

// Kappa is Cohen's kappa: agreement beyond what chance would give. It is
// undefined when chance agreement is already 1 (for example everyone says yes).
func (a Agreement) Kappa() (float64, bool) {
	return CohenKappa(a.YesYes, a.YesNo, a.NoYes, a.NoNo)
}

// FalsePositiveRate is how often the judge says yes when the person said no.
func (a Agreement) FalsePositiveRate() (float64, bool) {
	if a.NoYes+a.NoNo == 0 {
		return 0, false
	}
	return float64(a.NoYes) / float64(a.NoYes+a.NoNo), true
}

// FalseNegativeRate is how often the judge says no when the person said yes.
func (a Agreement) FalseNegativeRate() (float64, bool) {
	if a.YesYes+a.YesNo == 0 {
		return 0, false
	}
	return float64(a.YesNo) / float64(a.YesYes+a.YesNo), true
}

// CohenKappa computes kappa from a 2x2 table: yy is person yes and judge yes,
// yn is person yes and judge no, and so on.
func CohenKappa(yy, yn, ny, nn int) (float64, bool) {
	total := float64(yy + yn + ny + nn)
	if total == 0 {
		return 0, false
	}
	observed := float64(yy+nn) / total
	personYes := float64(yy+yn) / total
	judgeYes := float64(yy+ny) / total
	chance := personYes*judgeYes + (1-personYes)*(1-judgeYes)
	if chance >= 1 {
		return 0, false
	}
	return (observed - chance) / (1 - chance), true
}

// Calibration is the full comparison of a sheet against a report.
type Calibration struct {
	Correct   Agreement
	Supported Agreement
}

// Calibrate matches the sheet with the report by id and compares the grades.
// Report entries that are not in the sheet are normal (the sheet is a sample),
// so only sheet ids missing from the report are counted.
func Calibrate(report *AnswerReport, sheet []SheetEntry) Calibration {
	byID := map[string]AnswerResult{}
	for _, res := range report.Results {
		byID[res.ID] = res
	}

	c := Calibration{
		Correct:   Agreement{Metric: "correct", TotalSheet: len(sheet)},
		Supported: Agreement{Metric: "supported", TotalSheet: len(sheet)},
	}
	for _, entry := range sheet {
		res, found := byID[entry.ID]
		c.Correct.add(entry, entry.HumanCorrect, res.Correct, found)
		c.Supported.add(entry, entry.HumanSupported, res.Supported, found)
	}
	return c
}

func (a *Agreement) add(entry SheetEntry, human, judge *bool, found bool) {
	switch {
	case human == nil:
		a.Ungraded++
		return
	case !found:
		a.Graded++
		a.MissingID++
		return
	}
	a.Graded++
	if judge == nil {
		a.NoJudge++
		return
	}

	a.Compared++
	switch {
	case *human && *judge:
		a.YesYes++
	case *human && !*judge:
		a.YesNo++
	case !*human && *judge:
		a.NoYes++
	default:
		a.NoNo++
	}
	if *human != *judge {
		a.Disagreed = append(a.Disagreed, Disagreement{ID: entry.ID, Question: entry.Question})
	}
}

// RenderCalibration writes the comparison as Markdown.
func RenderCalibration(c Calibration) string {
	var b strings.Builder
	b.WriteString("# Judge calibration\n\n")
	c.Correct.render(&b, "Correct (same as the reference answer)")
	c.Supported.render(&b, "Supported (every claim is in the passages)")

	b.WriteString("## Disagreements\n\n")
	if len(c.Correct.Disagreed)+len(c.Supported.Disagreed) == 0 {
		b.WriteString("None.\n")
		return b.String()
	}
	writeDisagreements(&b, "correct", c.Correct.Disagreed)
	writeDisagreements(&b, "supported", c.Supported.Disagreed)
	return b.String()
}

func writeDisagreements(b *strings.Builder, metric string, list []Disagreement) {
	for _, d := range list {
		fmt.Fprintf(b, "- %s `%s`: %s\n", metric, d.ID, d.Question)
	}
}

func (a Agreement) render(b *strings.Builder, title string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	fmt.Fprintf(b, "- graded by the person: %d (ignored, left empty: %d)\n", a.Graded, a.Ungraded)
	fmt.Fprintf(b, "- also with a judge verdict (compared): %d\n", a.Compared)
	if a.NoJudge > 0 {
		fmt.Fprintf(b, "- skipped, the judge has no verdict: %d\n", a.NoJudge)
	}
	if a.MissingID > 0 {
		fmt.Fprintf(b, "- skipped, id not found in the answer report: %d\n", a.MissingID)
	}

	if a.Compared < minCalibrationSample {
		fmt.Fprintf(b, "\n> WARNING: only %d entries compared (fewer than %d). The sample is too small to trust these numbers.\n",
			a.Compared, minCalibrationSample)
	}
	if a.Compared == 0 {
		b.WriteString("\nNothing to compare.\n\n")
		return
	}

	b.WriteString("\n")
	fmt.Fprintf(b, "- agreement: %s\n", formatPercent(a.Percent()))
	fmt.Fprintf(b, "- Cohen's kappa: %s\n", formatKappa(a.Kappa()))
	fmt.Fprintf(b, "- judge false-positive rate (says yes, person said no): %s\n", formatPercent2(a.FalsePositiveRate()))
	fmt.Fprintf(b, "- judge false-negative rate (says no, person said yes): %s\n\n", formatPercent2(a.FalseNegativeRate()))

	b.WriteString("| | judge yes | judge no |\n|---|---|---|\n")
	fmt.Fprintf(b, "| person yes | %d | %d |\n", a.YesYes, a.YesNo)
	fmt.Fprintf(b, "| person no | %d | %d |\n\n", a.NoYes, a.NoNo)
}

func formatPercent(v float64, ok bool) string {
	if !ok {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", v)
}

// formatPercent2 formats a 0..1 rate as a percentage.
func formatPercent2(v float64, ok bool) string {
	return formatPercent(v*100, ok)
}

func formatKappa(v float64, ok bool) string {
	if !ok {
		return "n/a (undefined: chance agreement is already 100%, for example everyone said yes)"
	}
	return fmt.Sprintf("%.2f", v)
}
