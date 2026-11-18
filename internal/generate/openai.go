package generate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/piovani/RAGscope/internal/httpjson"
)

// DefaultOpenAIURL is the base URL of the OpenAI API. Any server that speaks the
// same Chat Completions protocol works by changing the base URL: Azure OpenAI,
// GitHub Models, OpenRouter, Groq, vLLM, LM Studio, llama.cpp, LocalAI and others.
const DefaultOpenAIURL = "https://api.openai.com/v1"

// OpenAI answers through the Chat Completions API (`POST {base}/chat/completions`).
// It is the de facto standard, so one adapter reaches most hosted and self-hosted
// models without tying the project to a single vendor.
type OpenAI struct {
	name        string
	model       string
	temperature float64
	inputPrice  float64 // USD per million input tokens
	outputPrice float64 // USD per million output tokens
	client      *httpjson.Client
}

// NewOpenAI builds the adapter. baseURL is the API root including the version,
// for example "https://api.openai.com/v1". apiKey may be empty for a local server
// that has no authentication.
func NewOpenAI(baseURL, apiKey, model string, temperature, inputPricePerMTok, outputPricePerMTok float64) *OpenAI {
	if baseURL == "" {
		baseURL = DefaultOpenAIURL
	}
	client := httpjson.New(baseURL)
	if apiKey != "" {
		client = client.WithHeader("Authorization", "Bearer "+apiKey)
	}
	return &OpenAI{
		name:        "openai/" + model,
		model:       model,
		temperature: temperature,
		inputPrice:  inputPricePerMTok,
		outputPrice: outputPricePerMTok,
		client:      client,
	}
}

func (o *OpenAI) Name() string { return o.name }

// Generate answers the question using only the passages in the request.
func (o *OpenAI) Generate(ctx context.Context, req Request) (Response, error) {
	system, user := buildPrompt(req)

	resp, err := o.Chat(ctx, system, user)
	if err != nil {
		return Response{}, err
	}
	resp.Refused = isRefusal(resp.Answer)
	return resp, nil
}

// Chat sends one system message and one user message and returns the reply.
// It is also used when the model acts as judge or query rewriter.
func (o *OpenAI) Chat(ctx context.Context, system, user string) (Response, error) {
	request := map[string]any{
		"model":       o.model,
		"temperature": o.temperature,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}

	var answer struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	start := time.Now()
	// The key is only in a header, so it never reaches the error.
	if err := o.client.Post(ctx, "/chat/completions", request, &answer); err != nil {
		return Response{}, fmt.Errorf("generate with %s: %w", o.model, err)
	}
	if len(answer.Choices) == 0 {
		return Response{}, fmt.Errorf("generate with %s: %w", o.model, errors.New("the server returned no choices"))
	}

	in, out := answer.Usage.PromptTokens, answer.Usage.CompletionTokens
	return Response{
		Answer: strings.TrimSpace(answer.Choices[0].Message.Content),
		Usage: Usage{
			InputTokens:  in,
			OutputTokens: out,
			Latency:      time.Since(start),
			CostUSD:      float64(in)/1e6*o.inputPrice + float64(out)/1e6*o.outputPrice,
		},
	}, nil
}
