// Package generate wraps LLM providers (Anthropic, OpenAI-compatible, Ollama).
package generate

import (
	"context"
	"time"

	"github.com/piovani/RAGscope/internal/domain"
)

// Request is a grounded-generation request.
type Request struct {
	Question string
	Context  []domain.ScoredChunk
}

// Usage records the cost-relevant facts of one generation call.
type Usage struct {
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
	CostUSD      float64 // 0 for local models
}

// Response is the model answer plus accounting data.
type Response struct {
	Answer  string
	Refused bool // the model declared the answer is not in the context
	Usage   Usage
}

// Generator produces an answer from a question and retrieved context.
type Generator interface {
	Name() string
	Generate(ctx context.Context, req Request) (Response, error)
}
