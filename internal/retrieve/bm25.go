package retrieve

import (
	"cmp"
	"context"
	"math"
	"slices"

	"github.com/piovani/RAGscope/internal/domain"
)

// The usual BM25 settings. k1 controls how fast repeating a word stops
// helping; b controls how much long chunks are penalized.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// BM25 ranks chunks by the words they share with the question. It lives in
// memory: build it once from all the chunks of a collection and then query it.
type BM25 struct {
	chunks     []domain.Chunk
	termCounts []map[string]int // for each chunk, how many times each term appears
	lengths    []int            // for each chunk, its number of terms
	docFreq    map[string]int   // in how many chunks each term appears
	avgLength  float64
}

func NewBM25(chunks []domain.Chunk) *BM25 {
	b := &BM25{
		chunks:     chunks,
		termCounts: make([]map[string]int, len(chunks)),
		lengths:    make([]int, len(chunks)),
		docFreq:    map[string]int{},
	}

	total := 0
	for i, chunk := range chunks {
		tokens := tokenize(chunk.Text)

		counts := map[string]int{}
		for _, token := range tokens {
			counts[token]++
		}
		for term := range counts {
			b.docFreq[term]++
		}

		b.termCounts[i] = counts
		b.lengths[i] = len(tokens)
		total += len(tokens)
	}

	if len(chunks) > 0 {
		b.avgLength = float64(total) / float64(len(chunks))
	}
	return b
}

func (b *BM25) Name() string { return "bm25" }

func (b *BM25) Retrieve(_ context.Context, query string, k int) ([]domain.ScoredChunk, error) {
	terms := tokenize(query)

	var results []domain.ScoredChunk
	for i, chunk := range b.chunks {
		if score := b.score(i, terms); score > 0 {
			results = append(results, domain.ScoredChunk{Chunk: chunk, Score: score})
		}
	}

	// Equal scores are ordered by ID so the result is always the same.
	slices.SortFunc(results, func(x, y domain.ScoredChunk) int {
		return cmp.Or(cmp.Compare(y.Score, x.Score), cmp.Compare(x.ID, y.ID))
	})
	if len(results) > k {
		results = results[:k]
	}
	return results, nil
}

func (b *BM25) score(i int, terms []string) float64 {
	n := float64(len(b.chunks))
	lengthRatio := float64(b.lengths[i]) / b.avgLength

	score := 0.0
	for _, term := range terms {
		tf := float64(b.termCounts[i][term])
		if tf == 0 {
			continue
		}

		// Rare terms count more than common ones.
		df := float64(b.docFreq[term])
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))

		score += idf * tf * (bm25K1 + 1) / (tf + bm25K1*(1-bm25B+bm25B*lengthRatio))
	}
	return score
}
