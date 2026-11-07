// Package api exposes the HTTP interface: a health check, the ask endpoint
// and a small web page to try it.
package api

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/piovani/RAGscope/internal/pipeline"
)

const (
	maxBodyBytes     = 4 << 10 // 4 KiB is plenty for one question
	maxQuestionChars = 500
	askTimeout       = 2 * time.Minute // a local model can be slow on a cold start
)

//go:embed web/index.html
var indexHTML []byte

// Asker answers a question. pipeline.Answerer implements it.
type Asker interface {
	Ask(ctx context.Context, question string) (pipeline.Answer, error)
}

// NewHandler returns the HTTP handler for the RAGscope API.
func NewHandler(asker Asker) http.Handler {
	s := &server{asker: asker}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("/api/ask", s.ask)
	return mux
}

type server struct {
	asker Asker
}

type askRequest struct {
	Question string `json:"question"`
}

type sourceJSON struct {
	N       int     `json:"n"`
	ID      string  `json:"id"`
	Article string  `json:"article"`
	Score   float64 `json:"score"`
	Text    string  `json:"text"`
}

type usageJSON struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	LatencyMS    int64   `json:"latency_ms"`
	CostUSD      float64 `json:"cost_usd"`
}

type askResponse struct {
	Answer  string       `json:"answer"`
	Refused bool         `json:"refused"`
	Sources []sourceJSON `json:"sources"`
	Usage   usageJSON    `json:"usage"`
}

func (s *server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (s *server) ask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	question, err := readQuestion(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), askTimeout)
	defer cancel()

	answer, err := s.asker.Ask(ctx, question)
	if err != nil {
		// The real cause stays in the log; the client gets nothing to dig into.
		slog.Error("ask failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not answer the question")
		return
	}
	writeJSON(w, http.StatusOK, toResponse(answer))
}

// readQuestion decodes and validates the request body.
func readQuestion(w http.ResponseWriter, r *http.Request) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return "", errors.New("invalid JSON body")
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return "", errors.New("question is required")
	}
	if utf8.RuneCountInString(question) > maxQuestionChars {
		return "", errors.New("question is too long (max 500 characters)")
	}
	return question, nil
}

func toResponse(a pipeline.Answer) askResponse {
	sources := make([]sourceJSON, 0, len(a.Sources))
	for i, s := range a.Sources {
		sources = append(sources, sourceJSON{
			N:       i + 1, // matches the [1], [2] citations in the answer
			ID:      s.ID,
			Article: s.Metadata["article"],
			Score:   s.Score,
			Text:    s.Text,
		})
	}
	return askResponse{
		Answer:  a.Text,
		Refused: a.Refused,
		Sources: sources,
		Usage: usageJSON{
			InputTokens:  a.Usage.InputTokens,
			OutputTokens: a.Usage.OutputTokens,
			LatencyMS:    a.Usage.Latency.Milliseconds(),
			CostUSD:      a.Usage.CostUSD,
		},
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
