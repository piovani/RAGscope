package eval

import (
	"math"
	"strings"

	"github.com/piovani/RAGscope/internal/domain"
)

// QuestionResult is what the retriever did for one question.
type QuestionResult struct {
	ID        string   `json:"id"`
	Question  string   `json:"question"`
	LatencyMS float64  `json:"latency_ms"`
	Ranks     []int    `json:"ranks"`     // one per evidence snippet; 0 = not retrieved
	Retrieved []string `json:"retrieved"` // chunk IDs in order, to help debugging
}

// findRanks looks, for each evidence snippet, at the position (starting at 1)
// of the first retrieved chunk that contains it. Zero means it was not found.
func findRanks(retrieved []domain.ScoredChunk, evidence []string) []int {
	texts := make([]string, len(retrieved))
	for i, chunk := range retrieved {
		texts[i] = normalize(chunk.Text)
	}

	ranks := make([]int, len(evidence))
	for i, snippet := range evidence {
		want := normalize(snippet)
		for pos, text := range texts {
			if strings.Contains(text, want) {
				ranks[i] = pos + 1
				break
			}
		}
	}
	return ranks
}

// Hit reports whether at least one evidence snippet is inside the top k.
func (r QuestionResult) Hit(k int) bool {
	for _, rank := range r.Ranks {
		if rank > 0 && rank <= k {
			return true
		}
	}
	return false
}

// Recall is the share of evidence snippets found inside the top k.
func (r QuestionResult) Recall(k int) float64 {
	if len(r.Ranks) == 0 {
		return 0
	}
	found := 0
	for _, rank := range r.Ranks {
		if rank > 0 && rank <= k {
			found++
		}
	}
	return float64(found) / float64(len(r.Ranks))
}

// ReciprocalRank is 1 divided by the position of the first relevant chunk
// (1 if it came first, 0.5 if second...). It is 0 when nothing relevant came back.
func (r QuestionResult) ReciprocalRank() float64 {
	return r.ReciprocalRankAt(math.MaxInt)
}

// ReciprocalRankAt is the same, but only looks at the first k positions.
func (r QuestionResult) ReciprocalRankAt(k int) float64 {
	best := 0
	for _, rank := range r.Ranks {
		if rank > 0 && rank <= k && (best == 0 || rank < best) {
			best = rank
		}
	}
	if best == 0 {
		return 0
	}
	return 1 / float64(best)
}
