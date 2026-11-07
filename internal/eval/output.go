package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Save writes the report as JSON in dir and returns the file path. The file
// name has the experiment name and the time, so runs never overwrite each other.
func (r *Report) Save(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	name := fmt.Sprintf("%s-%s.json", r.Experiment, r.RunAt.Format("20060102-150405"))
	path := filepath.Join(dir, name)

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

// Print shows a short summary for people reading the terminal.
func (r *Report) Print(w io.Writer) {
	fmt.Fprintf(w, "experiment: %s (retriever: %s)\n", r.Experiment, r.Retriever)
	fmt.Fprintf(w, "questions:  %d scored, %d unanswerable skipped (golden set %s)\n\n", r.Questions, r.Skipped, r.GoldenSet)

	fmt.Fprintf(w, "  top-k   hit rate   recall\n")
	for _, c := range r.Cutoffs {
		fmt.Fprintf(w, "  %5d   %8.3f   %6.3f\n", c.K, c.HitRate, c.Recall)
	}

	fmt.Fprintf(w, "\nMRR:     %.3f\n", r.MRR)
	fmt.Fprintf(w, "latency: mean %.0f ms, p95 %.0f ms\n", r.LatencyMean, r.LatencyP95)

	misses := r.Misses()
	if len(misses) == 0 {
		return
	}
	fmt.Fprintf(w, "\nnot found in the top %d (%d):\n", r.K, len(misses))
	for _, m := range misses {
		fmt.Fprintf(w, "  %s  %s\n", m.ID, m.Question)
	}
}

// Save writes the answer report as JSON in a "generation" folder inside dir. It
// is kept apart from the retrieval reports, which `ragscope report` compares.
func (r *AnswerReport) Save(dir string) (string, error) {
	dir = filepath.Join(dir, "generation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	name := fmt.Sprintf("%s-%s.json", r.Experiment, r.RunAt.Format("20060102-150405"))
	path := filepath.Join(dir, name)

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

// Print shows a short summary for people reading the terminal.
func (r *AnswerReport) Print(w io.Writer) {
	fmt.Fprintf(w, "experiment: %s (%s, retrieval: %s, golden set %s)\n", r.Experiment, r.Generator, r.Retriever, r.GoldenSet)
	fmt.Fprintf(w, "questions:  %d answerable, %d unanswerable\n\n", r.Answerable, r.Unanswerable)

	fmt.Fprintf(w, "  correct refusal   %.3f   (unanswerable questions the model refused)\n", r.CorrectRefusal)
	fmt.Fprintf(w, "  false refusal     %.3f   (answerable questions it refused by mistake)\n", r.FalseRefusal)
	fmt.Fprintf(w, "    evidence missing  %.3f   (the retriever did not bring the passage: refusing was fair)\n", r.FalseRefusalNoEvidence)
	fmt.Fprintf(w, "    evidence present  %.3f   (the passage was there and the model refused anyway)\n", r.FalseRefusalWithEvidence)
	fmt.Fprintf(w, "  with citation     %.3f   (answers that cite a passage like [1])\n", r.Citation)
	if r.CorrectJudged > 0 {
		fmt.Fprintf(w, "  correct           %.3f   (answerable questions answered like the reference; judged by a model, not calibrated)\n", r.Correct)
	}
	if r.Judged > 0 {
		fmt.Fprintf(w, "  faithfulness      %.3f   (%d answers judged by a model; not calibrated)\n", r.Faithfulness, r.Judged)
	}

	fmt.Fprintf(w, "\ntokens:  %d in, %d out\n", r.InputTokens, r.OutputTokens)
	if r.CostUSD > 0 {
		fmt.Fprintf(w, "cost:    US$ %.4f (US$ %.4f per question)\n", r.CostUSD, r.CostUSD/float64(len(r.Results)))
	}
	fmt.Fprintf(w, "latency: mean %.1f s, p95 %.1f s\n", r.LatencyMean/1000, r.LatencyP95/1000)

	var answeredButShouldNot, refusedWithEvidence, refusedWithout, unsupported, incorrect []AnswerResult
	for _, res := range r.Results {
		switch {
		case res.Unanswerable && !res.Refused:
			answeredButShouldNot = append(answeredButShouldNot, res)
		case !res.Unanswerable && res.Refused && res.EvidenceRetrieved != nil && *res.EvidenceRetrieved:
			refusedWithEvidence = append(refusedWithEvidence, res)
		case !res.Unanswerable && res.Refused:
			refusedWithout = append(refusedWithout, res)
		}
		if res.Supported != nil && !*res.Supported {
			unsupported = append(unsupported, res)
		}
		if res.Correct != nil && !*res.Correct {
			incorrect = append(incorrect, res)
		}
	}
	printList(w, "answered, but the law has no answer", answeredButShouldNot)
	printList(w, "refused although the passage was there (the model was too timid)", refusedWithEvidence)
	printList(w, "refused because the retriever missed the passage", refusedWithout)
	printList(w, "answer judged different from the reference", incorrect)
	printList(w, "answer not supported by the passages (judge)", unsupported)
}

func printList(w io.Writer, title string, results []AnswerResult) {
	if len(results) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s (%d):\n", title, len(results))
	for _, res := range results {
		fmt.Fprintf(w, "  %s  %s\n", res.ID, res.Question)
	}
}
