package report

import (
	"cmp"
	"fmt"
	"io"
	"slices"

	"github.com/piovani/RAGscope/internal/eval"
)

// Markdown writes a comparison table, best experiment first. When baseline is
// not nil, two columns show how far each experiment is from it, with a 95%
// confidence interval so you can tell a real gain from luck.
func Markdown(w io.Writer, reports []*eval.Report, baseline *eval.Report) {
	ranked := slices.Clone(reports)
	slices.SortFunc(ranked, func(a, b *eval.Report) int {
		return cmp.Or(
			cmp.Compare(b.Final().Recall, a.Final().Recall), // best recall first
			cmp.Compare(b.MRR, a.MRR),
			cmp.Compare(a.Experiment, b.Experiment),
		)
	})

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
	for _, r := range ranked {
		final := r.Final()
		fmt.Fprintf(w, "| %s | %s | %d | %.3f | %.3f | %.3f | %.0f |",
			r.Experiment, r.Retriever, final.K, final.HitRate, final.Recall, r.MRR, r.LatencyP95)

		if baseline != nil {
			if r == baseline {
				fmt.Fprint(w, " baseline | baseline |")
			} else {
				recall, recallSig := delta(pairedDiffs(r, baseline, recallScore), final.Recall-baseline.Final().Recall)
				mrr, mrrSig := delta(pairedDiffs(r, baseline, mrrScore), r.MRR-baseline.MRR)
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
}
