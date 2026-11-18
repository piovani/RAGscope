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

const openAIKey = "sk-test-key"

func fakeOpenAI(t *testing.T, key string, status int, body string) (*OpenAI, *http.Request, *[]byte) {
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
	return NewOpenAI(server.URL+"/v1", key, "gpt-test", 0.2, 3, 15), &gotReq, &gotBody
}

const openAIOK = `{"choices":[{"message":{"role":"assistant","content":"  Resposta completa [1]. \n"}}],
"usage":{"prompt_tokens":2000,"completion_tokens":500}}`

func TestOpenAISendsAuthAndBody(t *testing.T) {
	o, req, body := fakeOpenAI(t, openAIKey, 200, openAIOK)

	if _, err := o.Chat(context.Background(), "be brief", "hello"); err != nil {
		t.Fatal(err)
	}

	if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions" {
		t.Errorf("request = %s %s", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer "+openAIKey {
		t.Errorf("Authorization = %q", got)
	}
	if strings.Contains(string(*body), openAIKey) {
		t.Error("the key must not be in the body")
	}

	var sent struct {
		Model       string              `json:"model"`
		Temperature float64             `json:"temperature"`
		Messages    []map[string]string `json:"messages"`
	}
	if err := json.Unmarshal(*body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Model != "gpt-test" || sent.Temperature != 0.2 {
		t.Errorf("body = %+v", sent)
	}
	if len(sent.Messages) != 2 || sent.Messages[0]["role"] != "system" || sent.Messages[1]["role"] != "user" {
		t.Errorf("messages = %v", sent.Messages)
	}
}

func TestOpenAINoKeyMeansNoAuthHeader(t *testing.T) {
	o, req, _ := fakeOpenAI(t, "", 200, openAIOK)
	if _, err := o.Chat(context.Background(), "s", "u"); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("a local server without a key must get no Authorization header, got %q", got)
	}
}

func TestOpenAIAnswerAndCost(t *testing.T) {
	o, _, _ := fakeOpenAI(t, openAIKey, 200, openAIOK)

	resp, err := o.Generate(context.Background(), Request{
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
	// 2000/1e6*3 + 500/1e6*15
	if want := 0.0135; math.Abs(resp.Usage.CostUSD-want) > 1e-12 {
		t.Errorf("cost = %v, want %v", resp.Usage.CostUSD, want)
	}
	if o.Name() != "openai/gpt-test" {
		t.Errorf("name = %q", o.Name())
	}
}

func TestOpenAIMarksRefusal(t *testing.T) {
	o, _, _ := fakeOpenAI(t, "", 200, `{"choices":[{"message":{"content":"SEM_RESPOSTA"}}],"usage":{}}`)
	resp, err := o.Generate(context.Background(), Request{Question: "?"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Refused {
		t.Error("Refused should be true")
	}
}

func TestOpenAINoChoices(t *testing.T) {
	o, _, _ := fakeOpenAI(t, "", 200, `{"choices":[]}`)
	if _, err := o.Chat(context.Background(), "s", "u"); err == nil {
		t.Fatal("expected an error when the server returns no choices")
	}
}

func TestOpenAIErrorHasAPIMessageButNotTheKey(t *testing.T) {
	o, _, _ := fakeOpenAI(t, openAIKey, 429, `{"error":{"message":"too many requests"}}`)

	_, err := o.Chat(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "too many requests") {
		t.Errorf("error should carry the API message: %v", err)
	}
	if strings.Contains(err.Error(), openAIKey) {
		t.Errorf("error leaks the key: %v", err)
	}
}

func TestOpenAIDefaultBaseURL(t *testing.T) {
	// Only checks the default is applied; no request is made.
	if o := NewOpenAI("", "k", "m", 0, 0, 0); o == nil {
		t.Fatal("nil adapter")
	}
}
