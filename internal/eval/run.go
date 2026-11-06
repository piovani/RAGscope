package eval

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/piovani/RAGscope/internal/retrieve"
)

// CutoffScore holds the averages for the top k results.
type CutoffScore struct {
	K       int     `json:"k"`
	HitRate float64 `json:"hit_rate"` // share of questions with at least one relevant chunk in the top k
	Recall  float64 `json:"recall"`   // average share of evidence found in the top k
}

// Report is the outcome of running one experiment against the golden set.
type Report struct {
	Experiment  string           `json:"experiment"`
	Retriever   string           `json:"retriever"`
	K           int              `json:"k"`
	RunAt       time.Time        `json:"run_at"`
	GoldenSet   string           `json:"golden_set,omitempty"` // Fingerprint of the questions used
	Questions   int              `json:"questions"`            // answerable questions that were scored
	Skipped     int              `json:"skipped"`              // unanswerable ones, not used in retrieval metrics
	Cutoffs     []CutoffScore    `json:"cutoffs"`
	MRR         float64          `json:"mrr"`
	LatencyMean float64          `json:"latency_mean_ms"`
	LatencyP95  float64          `json:"latency_p95_ms"`
	Results     []QuestionResult `json:"results"`
}

// Final returns the scores at the report's own k, the one that counts.
func (r *Report) Final() CutoffScore {
	if len(r.Cutoffs) == 0 {
		return CutoffScore{K: r.K}
	}
	return r.Cutoffs[len(r.Cutoffs)-1]
}

// Misses returns the questions where no evidence was found in the top k.
func (r *Report) Misses() []QuestionResult {
	var misses []QuestionResult
	for _, res := range r.Results {
		if !res.Hit(r.K) {
			misses = append(misses, res)
		}
	}
	return misses
}

// RunRetrieval asks the retriever for the top k chunks of every answerable
// question and scores what came back.
func RunRetrieval(ctx context.Context, retriever retrieve.Retriever, questions []Question, k int) (*Report, error) {
	report := &Report{Retriever: retriever.Name(), K: k, RunAt: time.Now()}

	for _, q := range questions {
		if q.Unanswerable {
			report.Skipped++
			continue
		}

		start := time.Now()
		retrieved, err := retriever.Retrieve(ctx, q.Question, k)
		if err != nil {
			return nil, fmt.Errorf("question %s: %w", q.ID, err)
		}

		ids := make([]string, len(retrieved))
		for i, chunk := range retrieved {
			ids[i] = chunk.ID
		}

		report.Results = append(report.Results, QuestionResult{
			ID:        q.ID,
			Question:  q.Question,
			LatencyMS: float64(time.Since(start)) / float64(time.Millisecond),
			Ranks:     findRanks(retrieved, q.Evidence),
			Retrieved: ids,
		})
	}

	report.Questions = len(report.Results)
	if report.Questions == 0 {
		return nil, fmt.Errorf("the golden set has no answerable questions")
	}
	report.summarize()
	return report, nil
}

func (r *Report) summarize() {
	n := float64(len(r.Results))

	for _, k := range cutoffs(r.K) {
		score := CutoffScore{K: k}
		for _, res := range r.Results {
			if res.Hit(k) {
				score.HitRate++
			}
			score.Recall += res.Recall(k)
		}
		score.HitRate /= n
		score.Recall /= n
		r.Cutoffs = append(r.Cutoffs, score)
	}

	latencies := make([]float64, len(r.Results))
	for i, res := range r.Results {
		r.MRR += res.ReciprocalRank()
		r.LatencyMean += res.LatencyMS
		latencies[i] = res.LatencyMS
	}
	r.MRR /= n
	r.LatencyMean /= n

	slices.Sort(latencies)
	r.LatencyP95 = latencies[int(math.Ceil(0.95*n))-1]
}

// cutoffs picks the k values to report: the usual 1, 3, 5, 10 that fit, plus k itself.
func cutoffs(k int) []int {
	var out []int
	for _, c := range []int{1, 3, 5, 10} {
		if c < k {
			out = append(out, c)
		}
	}
	return append(out, k)
}
