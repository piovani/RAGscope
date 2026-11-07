package generate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/piovani/RAGscope/internal/httpjson"
)

// Five passages of 512 words do not fit in Ollama's default context window and
// would be cut silently, so we ask for a larger one.
const contextWindow = 8192

// Ollama answers with a model served by a local Ollama.
type Ollama struct {
	model       string
	temperature float64
	client      *httpjson.Client
}

func NewOllama(url, model string, temperature float64) *Ollama {
	return &Ollama{model: model, temperature: temperature, client: httpjson.New(url)}
}

func (o *Ollama) Name() string { return "ollama/" + o.model }

// Generate answers the question using only the passages in the request.
func (o *Ollama) Generate(ctx context.Context, req Request) (Response, error) {
	system, user := buildPrompt(req)

	resp, err := o.Chat(ctx, system, user)
	if err != nil {
		return Response{}, err
	}
	resp.Refused = isRefusal(resp.Answer)
	return resp, nil
}

// Chat sends one system message and one user message and returns the reply.
// It is also used by the answer judge, which has its own prompts.
func (o *Ollama) Chat(ctx context.Context, system, user string) (Response, error) {
	request := map[string]any{
		"model":  o.model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"options": map[string]any{
			"temperature": o.temperature,
			"num_ctx":     contextWindow,
		},
	}

	var answer struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		PromptTokens int `json:"prompt_eval_count"`
		OutputTokens int `json:"eval_count"`
	}

	start := time.Now()
	if err := o.client.Post(ctx, "/api/chat", request, &answer); err != nil {
		return Response{}, fmt.Errorf("generate with %s: %w", o.model, err)
	}

	return Response{
		Answer: strings.TrimSpace(answer.Message.Content),
		Usage: Usage{
			InputTokens:  answer.PromptTokens,
			OutputTokens: answer.OutputTokens,
			Latency:      time.Since(start),
		},
	}, nil
}
