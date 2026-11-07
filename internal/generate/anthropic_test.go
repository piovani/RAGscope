package generate

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

const testKey = "sk-ant-test-key"

// fakeAnthropic answers every call with status and body, and keeps what it received.
func fakeAnthropic(t *testing.T, status int, body string) (*Anthropic, *http.Request, *[]byte) {
	t.Helper()
	var gotReq http.Request
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = *r.Clone(context.Background())
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return NewAnthropicAt(server.URL, testKey, "claude-test", 0.2, 3, 15), &gotReq, &gotBody
}

const okBody = `{"content":[{"type":"text","text":"  Resposta "},{"type":"tool_use","text":"ignored"},{"type":"text","text":"completa [1]. \n"}],
"usage":{"input_tokens":2000,"output_tokens":500}}`

func TestAnthropicSendsHeadersAndBody(t *testing.T) {
	a, req, body := fakeAnthropic(t, 200, okBody)

	if _, err := a.Chat(context.Background(), "be brief", "hello"); err != nil {
		t.Fatal(err)
	}

	if req.Method != http.MethodPost || req.URL.Path != "/v1/messages" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	for header, want := range map[string]string{
		"x-api-key":         testKey,
		"anthropic-version": "2023-06-01",
		"content-type":      "application/json",
	} {
		if got := req.Header.Get(header); got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}
	if strings.Contains(string(*body), testKey) {
		t.Error("the key must not be in the body")
	}

	var sent struct {
		Model       string              `json:"model"`
		MaxTokens   int                 `json:"max_tokens"`
		Temperature float64             `json:"temperature"`
		System      string              `json:"system"`
		Messages    []map[string]string `json:"messages"`
	}
	if err := json.Unmarshal(*body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Model != "claude-test" || sent.MaxTokens != 1024 || sent.Temperature != 0.2 || sent.System != "be brief" {
		t.Errorf("body = %+v", sent)
	}
	if len(sent.Messages) != 1 || sent.Messages[0]["role"] != "user" || sent.Messages[0]["content"] != "hello" {
		t.Errorf("messages = %v, want only the user message", sent.Messages)
	}
}

func TestAnthropicJoinsTextBlocksAndComputesCost(t *testing.T) {
	a, _, _ := fakeAnthropic(t, 200, okBody)

	resp, err := a.Generate(context.Background(), Request{
		Question: "O que é dado pessoal?",
		Context:  []domain.ScoredChunk{passage("lgpd-0006", "5", "texto")},
	})
	if err != nil {
		t.Fatal(err)
	}

	if want := "Resposta completa [1]."; resp.Answer != want {
		t.Errorf("answer = %q, want %q", resp.Answer, want)
	}
	if resp.Refused {
		t.Error("a normal answer must not be a refusal")
	}
	if resp.Usage.InputTokens != 2000 || resp.Usage.OutputTokens != 500 {
		t.Errorf("tokens = %+v", resp.Usage)
	}
	// 2000/1e6*3 + 500/1e6*15 = 0.006 + 0.0075
	if want := 0.0135; math.Abs(resp.Usage.CostUSD-want) > 1e-12 {
		t.Errorf("cost = %v, want %v", resp.Usage.CostUSD, want)
	}
	if a.Name() != "anthropic/claude-test" {
		t.Errorf("name = %q", a.Name())
	}
}

func TestAnthropicMarksRefusal(t *testing.T) {
	a, _, _ := fakeAnthropic(t, 200, `{"content":[{"type":"text","text":"SEM_RESPOSTA"}],"usage":{"input_tokens":1,"output_tokens":1}}`)

	resp, err := a.Generate(context.Background(), Request{Question: "?"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Refused {
		t.Error("Refused should be true")
	}
}

func TestAnthropicErrorHasAPIMessageButNotTheKey(t *testing.T) {
	a, _, _ := fakeAnthropic(t, 429, `{"type":"error","error":{"type":"rate_limit_error","message":"too many requests"}}`)

	_, err := a.Chat(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "too many requests") {
		t.Errorf("error should carry the API message: %v", err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("error leaks the key: %v", err)
	}
}
