package pipeline

import (
	"context"
	"errors"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/generate"
	"github.com/piovani/RAGscope/internal/retrieve"
)

// Answerer answers one question: it finds the passages and asks the model.
// The CLI and the HTTP API share it, so both behave the same way.
type Answerer struct {
	Retriever retrieve.Retriever
	Generator generate.Generator
	K         int
}

// Answer is what the user sees: the text and the passages behind it.
type Answer struct {
	Text    string
	Refused bool
	Sources []domain.ScoredChunk // in the order the model saw them, so [1], [2] match
	Usage   generate.Usage
}

// Ask retrieves K passages and asks the generator. When nothing is retrieved
// it refuses right away: calling the model with no context would only invite
// an invented answer (and cost time).
// ErrNoGenerator is returned by Ask when the Answerer was built for search only.
var ErrNoGenerator = errors.New("no generation model is configured (retrieval-only mode)")

func (a Answerer) Ask(ctx context.Context, question string) (Answer, error) {
	if a.Generator == nil {
		return Answer{}, ErrNoGenerator
	}
	sources, err := a.Retriever.Retrieve(ctx, question, a.K)
	if err != nil {
		return Answer{}, err
	}
	if len(sources) == 0 {
		return Answer{Refused: true}, nil
	}

	resp, err := a.Generator.Generate(ctx, generate.Request{Question: question, Context: sources})
	if err != nil {
		return Answer{}, err
	}
	return Answer{Text: resp.Answer, Refused: resp.Refused, Sources: sources, Usage: resp.Usage}, nil
}

// Search returns the passages for a question and calls no model. It is how an
// outside assistant (ChatGPT, Copilot, Claude or any other) uses RAGscope: it
// brings its own model and only needs the evidence.
func (a Answerer) Search(ctx context.Context, query string, k int) ([]domain.ScoredChunk, error) {
	if k <= 0 {
		k = a.K
	}
	return a.Retriever.Retrieve(ctx, query, k)
}
