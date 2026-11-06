// Package report compares the results saved by `ragscope eval`.
package report

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/piovani/RAGscope/internal/eval"
)

// Load reads every report saved in dir.
func Load(dir string) ([]*eval.Report, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no results found in %s. Run `ragscope eval` first", dir)
	}

	reports := make([]*eval.Report, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		var r eval.Report
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		reports = append(reports, &r)
	}
	return reports, nil
}

// Latest keeps only the most recent run of each experiment, ordered by name.
func Latest(reports []*eval.Report) []*eval.Report {
	newest := map[string]*eval.Report{}
	for _, r := range reports {
		if current, ok := newest[r.Experiment]; !ok || r.RunAt.After(current.RunAt) {
			newest[r.Experiment] = r
		}
	}

	latest := make([]*eval.Report, 0, len(newest))
	for _, r := range newest {
		latest = append(latest, r)
	}
	slices.SortFunc(latest, func(a, b *eval.Report) int {
		return cmp.Compare(a.Experiment, b.Experiment)
	})
	return latest
}

// Find returns the report of the experiment with the given name, or nil.
func Find(reports []*eval.Report, experiment string) *eval.Report {
	for _, r := range reports {
		if r.Experiment == experiment {
			return r
		}
	}
	return nil
}

// Warnings lists reasons why the reports may not be fairly comparable.
func Warnings(reports []*eval.Report) []string {
	var warnings []string

	if values := distinct(reports, func(r *eval.Report) int { return r.K }); len(values) > 1 {
		warnings = append(warnings, fmt.Sprintf("the experiments use different k (%v), so hit rate and recall are not directly comparable", values))
	}
	if values := distinct(reports, func(r *eval.Report) int { return r.Questions }); len(values) > 1 {
		warnings = append(warnings, fmt.Sprintf("the experiments were scored on different numbers of questions (%v). Was the golden set changed?", values))
	}
	// Reports from before the golden set was fingerprinted have none; skip them.
	var versions []string
	for _, r := range reports {
		if r.GoldenSet != "" && !slices.Contains(versions, r.GoldenSet) {
			versions = append(versions, r.GoldenSet)
		}
	}
	if len(versions) > 1 {
		slices.Sort(versions)
		warnings = append(warnings, fmt.Sprintf("the experiments were scored on different versions of the golden set (%v). Run them again on the current one", versions))
	}
	return warnings
}

// distinct returns the different values, sorted, that get returns for the reports.
func distinct(reports []*eval.Report, get func(*eval.Report) int) []int {
	var values []int
	for _, r := range reports {
		if v := get(r); !slices.Contains(values, v) {
			values = append(values, v)
		}
	}
	slices.Sort(values)
	return values
}
