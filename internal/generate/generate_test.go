package generate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

func passage(id, article, text string) domain.ScoredChunk {
	chunk := domain.Chunk{ID: id, Text: text}
	if article != "" {
		chunk.Metadata = map[string]string{"article": article}
	}
	return domain.ScoredChunk{Chunk: chunk}
}

func TestBuildPromptNumbersThePassages(t *testing.T) {
	system, user := buildPrompt(Request{
		Question: "O que é dado pessoal?",
		Context: []domain.ScoredChunk{
			passage("lgpd-0006", "5", "dado pessoal: informação relacionada a pessoa natural"),
			passage("lgpd-0009", "", "texto sem artigo"),
		},
	})

	for _, want := range []string{
		"[1] (Art. 5, trecho lgpd-0006)",
		"dado pessoal: informação relacionada a pessoa natural",
		"[2] (trecho lgpd-0009)", // no article known: falls back to the chunk id
		"Pergunta: O que é dado pessoal?",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt is missing %q:\n%s", want, user)
		}
	}
	if !strings.Contains(system, RefusalMarker) {
		t.Error("the system prompt must tell the model how to refuse")
	}
}

func TestIsRefusal(t *testing.T) {
	tests := []struct {
		answer string
		want   bool
	}{
		{"SEM_RESPOSTA", true},
		{"SEM_RESPOSTA.", true},
		{"sem_resposta", true},
		{"Os trechos não trazem isso. SEM_RESPOSTA", true},
		{"O prazo é de 15 dias [1].", false},
		{"", false},
		// A long real answer that happens to mention the marker is not a refusal.
		{strings.Repeat("Texto longo da resposta. ", 10) + "SEM_RESPOSTA", false},
	}
	for _, tt := range tests {
		if got := isRefusal(tt.answer); got != tt.want {
			t.Errorf("isRefusal(%.40q) = %v, want %v", tt.answer, got, tt.want)
		}
	}
}

func TestOllamaGenerate(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		json.NewDecoder(r.Body).Decode(&sent)
		json.NewEncoder(w).Encode(map[string]any{
			"message":           map[string]string{"role": "assistant", "content": "  É informação sobre pessoa natural [1].  "},
			"prompt_eval_count": 321,
			"eval_count":        17,
		})
	}))
	defer srv.Close()

	resp, err := NewOllama(srv.URL, "llama3.2:3b", 0).Generate(context.Background(), Request{
		Question: "O que é dado pessoal?",
		Context:  []domain.ScoredChunk{passage("c1", "5", "texto")},
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Answer != "É informação sobre pessoa natural [1]." {
		t.Errorf("answer = %q (should be trimmed)", resp.Answer)
	}
	if resp.Refused {
		t.Error("a normal answer must not be marked as a refusal")
	}
	if resp.Usage.InputTokens != 321 || resp.Usage.OutputTokens != 17 {
		t.Errorf("usage = %+v", resp.Usage)
	}

	if sent["model"] != "llama3.2:3b" || sent["stream"] != false {
		t.Errorf("request = %v", sent)
	}
	messages := sent["messages"].([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" {
		t.Errorf("messages = %v", messages)
	}
	if opts := sent["options"].(map[string]any); opts["num_ctx"] != float64(contextWindow) {
		t.Errorf("options = %v, want a bigger context window", opts)
	}
}

func TestOllamaGenerateMarksRefusals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": "SEM_RESPOSTA"}})
	}))
	defer srv.Close()

	resp, err := NewOllama(srv.URL, "m", 0).Generate(context.Background(), Request{Question: "q"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Refused {
		t.Error("expected the answer to be marked as a refusal")
	}
}

func TestOllamaReportsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `model "x" not found`, http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := NewOllama(srv.URL, "x", 0).Generate(context.Background(), Request{Question: "q"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want the server message", err)
	}
}
