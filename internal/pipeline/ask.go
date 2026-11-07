package pipeline

import (
	"context"

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
func (a Answerer) Ask(ctx context.Context, question string) (Answer, error) {
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
