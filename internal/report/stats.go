package report

import (
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/piovani/RAGscope/internal/eval"
)

// With only a few dozen questions, a difference between two experiments can
// easily be luck. The bootstrap asks: if we had drawn the questions again, how
// much would the difference move?
const resamples = 5000

// bootstrapMean returns the mean of values and a 95% confidence interval for
// it, found by drawing the values again and again (with repetition).
func bootstrapMean(values []float64) (mean, low, high float64) {
	n := len(values)
	if n == 0 {
		return 0, 0, 0
	}

	// A fixed seed makes the same data always give the same interval.
	rng := rand.New(rand.NewPCG(1, 2))

	means := make([]float64, resamples)
	for i := range means {
		sum := 0.0
		for range n {
			sum += values[rng.IntN(n)]
		}
		means[i] = sum / float64(n)
	}
	slices.Sort(means)

	for _, v := range values {
		mean += v
	}
	mean /= float64(n)

	return mean, means[int(0.025*resamples)], means[int(0.975*resamples)-1]
}

// pairedDiffs compares two experiments question by question. Both answered the
// same questions, so for each one we take how much better exp did than base.
// Questions only one of them scored are ignored.
func pairedDiffs(exp, base *eval.Report, score func(res eval.QuestionResult, k int) float64) []float64 {
	baseByID := make(map[string]eval.QuestionResult, len(base.Results))
	for _, res := range base.Results {
		baseByID[res.ID] = res
	}

	var diffs []float64
	for _, res := range exp.Results {
		if baseRes, ok := baseByID[res.ID]; ok {
			diffs = append(diffs, score(res, exp.K)-score(baseRes, base.K))
		}
	}
	return diffs
}

func recallScore(res eval.QuestionResult, k int) float64 { return res.Recall(k) }
func mrrScore(res eval.QuestionResult, _ int) float64    { return res.ReciprocalRank() }

// delta formats a difference to the baseline, like "+0.200 [+0.067, +0.333] *".
// The star means the interval does not include zero, so the difference is
// unlikely to be luck. Without per-question data it shows the plain number.
func delta(diffs []float64, fallback float64) (text string, significant bool) {
	if len(diffs) == 0 {
		return fmt.Sprintf("%+.3f", fallback), false
	}

	mean, low, high := bootstrapMean(diffs)
	significant = low > 0 || high < 0

	text = fmt.Sprintf("%+.3f [%+.3f, %+.3f]", mean, low, high)
	if significant {
		text += " *"
	}
	return text, significant
}
