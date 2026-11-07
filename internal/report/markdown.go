package report

import (
	"cmp"
	"fmt"
	"io"
	"slices"

	"github.com/piovani/RAGscope/internal/eval"
)

// Scores are the averages shown for one experiment in the table.
type Scores struct {
	K       int
	HitRate float64
	Recall  float64
	MRR     float64
}

// ScoresAt returns the averages looking only at the first k positions. With
// k = 0 it returns the numbers saved in the report, at the experiment's own k.
func ScoresAt(r *eval.Report, k int) (Scores, error) {
	if k == 0 || len(r.Results) == 0 {
		final := r.Final()
		return Scores{K: final.K, HitRate: final.HitRate, Recall: final.Recall, MRR: r.MRR}, nil
	}
	if k > r.K {
		return Scores{}, fmt.Errorf("%s retrieved only %d chunks per question, so it has no top-%d", r.Experiment, r.K, k)
	}

	s := Scores{K: k}
	n := float64(len(r.Results))
	for _, res := range r.Results {
		if res.Hit(k) {
			s.HitRate++
		}
		s.Recall += res.Recall(k)
		s.MRR += res.ReciprocalRankAt(k)
	}
	s.HitRate /= n
	s.Recall /= n
	s.MRR /= n
	return s, nil
}

// Markdown writes a comparison table, best experiment first. When baseline is
// not nil, two columns show how far each experiment is from it, with a 95%
// confidence interval so you can tell a real gain from luck.
//
// k is how many positions to look at; 0 means each experiment's own k.
func Markdown(w io.Writer, reports []*eval.Report, baseline *eval.Report, k int) error {
	type row struct {
		report *eval.Report
		scores Scores
	}
	rows := make([]row, 0, len(reports))
	for _, r := range reports {
		scores, err := ScoresAt(r, k)
		if err != nil {
			return err
		}
		rows = append(rows, row{r, scores})
	}
	slices.SortFunc(rows, func(a, b row) int {
		return cmp.Or(
			cmp.Compare(b.scores.Recall, a.scores.Recall), // best recall first
			cmp.Compare(b.scores.MRR, a.scores.MRR),
			cmp.Compare(a.report.Experiment, b.report.Experiment),
		)
	})

	var baseScores Scores
	if baseline != nil {
		var err error
		if baseScores, err = ScoresAt(baseline, k); err != nil {
			return err
		}
	}

	fmt.Fprint(w, "| Experiment | Retriever | k | Hit rate | Recall | MRR | p95 (ms) |")
	if baseline != nil {
		fmt.Fprint(w, " Δ Recall [95% CI] | Δ MRR [95% CI] |")
	}
	fmt.Fprintln(w)

	fmt.Fprint(w, "|---|---|--:|--:|--:|--:|--:|")
	if baseline != nil {
		fmt.Fprint(w, "--:|--:|")
	}
	fmt.Fprintln(w)

	anyDelta, anySignificant := false, false
	for _, row := range rows {
		r, s := row.report, row.scores
		fmt.Fprintf(w, "| %s | %s | %d | %.3f | %.3f | %.3f | %.0f |",
			r.Experiment, r.Retriever, s.K, s.HitRate, s.Recall, s.MRR, r.LatencyP95)

		if baseline != nil {
			if r == baseline {
				fmt.Fprint(w, " baseline | baseline |")
			} else {
				recall, recallSig := delta(pairedDiffs(r, baseline, k, recallScore), s.Recall-baseScores.Recall)
				mrr, mrrSig := delta(pairedDiffs(r, baseline, k, mrrScore), s.MRR-baseScores.MRR)
				fmt.Fprintf(w, " %s | %s |", recall, mrr)

				anyDelta = true
				anySignificant = anySignificant || recallSig || mrrSig
			}
		}
		fmt.Fprintln(w)
	}

	if anyDelta {
		fmt.Fprintln(w, "\n> Δ is the mean difference to the baseline over the same questions, with a 95% bootstrap interval.")
		if anySignificant {
			fmt.Fprintln(w, "> \\* The interval does not include 0, so the difference is unlikely to be luck.")
		}
	}
	for _, warning := range Warnings(reports) {
		fmt.Fprintf(w, "\n> **Warning:** %s.\n", warning)
	}
	return nil
}
