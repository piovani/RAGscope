package generate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/piovani/RAGscope/internal/httpjson"
)

const (
	anthropicURL     = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
	// Answers are short and cited; this is a ceiling, not a target.
	anthropicMaxTokens = 1024
)

// Anthropic answers with a Claude model through the Messages API.
type Anthropic struct {
	model       string
	temperature float64
	inputPrice  float64 // USD per million input tokens
	outputPrice float64 // USD per million output tokens
	client      *httpjson.Client
}

func NewAnthropic(apiKey, model string, temperature, inputPricePerMTok, outputPricePerMTok float64) *Anthropic {
	return NewAnthropicAt(anthropicURL, apiKey, model, temperature, inputPricePerMTok, outputPricePerMTok)
}

// NewAnthropicAt is NewAnthropic with another base URL, used by tests.
func NewAnthropicAt(baseURL, apiKey, model string, temperature, inputPricePerMTok, outputPricePerMTok float64) *Anthropic {
	client := httpjson.New(baseURL).
		WithHeader("x-api-key", apiKey).
		WithHeader("anthropic-version", anthropicVersion)

	return &Anthropic{
		model:       model,
		temperature: temperature,
		inputPrice:  inputPricePerMTok,
		outputPrice: outputPricePerMTok,
		client:      client,
	}
}

func (a *Anthropic) Name() string { return "anthropic/" + a.model }

// Generate answers the question using only the passages in the request.
func (a *Anthropic) Generate(ctx context.Context, req Request) (Response, error) {
	system, user := buildPrompt(req)

	resp, err := a.Chat(ctx, system, user)
	if err != nil {
		return Response{}, err
	}
	resp.Refused = isRefusal(resp.Answer)
	return resp, nil
}

// Chat sends one system message and one user message and returns the reply.
// It is also used when the model acts as judge or query rewriter.
func (a *Anthropic) Chat(ctx context.Context, system, user string) (Response, error) {
	// Unlike Ollama, the system prompt is a field of its own, not a message.
	request := map[string]any{
		"model":       a.model,
		"max_tokens":  anthropicMaxTokens,
		"temperature": a.temperature,
		"system":      system,
		"messages": []map[string]string{
			{"role": "user", "content": user},
		},
	}

	var answer struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	start := time.Now()
	// On an error status httpjson already reports the status and the start of
	// the body, which holds the API's own message. The key is only in a header,
	// so it never reaches the error.
	if err := a.client.Post(ctx, "/v1/messages", request, &answer); err != nil {
		return Response{}, fmt.Errorf("generate with %s: %w", a.model, err)
	}

	var text strings.Builder
	for _, block := range answer.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}

	in, out := answer.Usage.InputTokens, answer.Usage.OutputTokens
	return Response{
		Answer: strings.TrimSpace(text.String()),
		Usage: Usage{
			InputTokens:  in,
			OutputTokens: out,
			Latency:      time.Since(start),
			CostUSD:      float64(in)/1e6*a.inputPrice + float64(out)/1e6*a.outputPrice,
		},
	}, nil
}
