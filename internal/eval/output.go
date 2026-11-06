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
	fmt.Fprintf(w, "questions:  %d scored, %d unanswerable skipped\n\n", r.Questions, r.Skipped)

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
