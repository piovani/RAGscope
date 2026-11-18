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

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/pipeline"
)

const (
	maxBodyBytes     = 4 << 10 // 4 KiB is plenty for one question
	maxQuestionChars = 500
	askTimeout       = 2 * time.Minute // a local model can be slow on a cold start
)

//go:embed web/index.html
var indexHTML []byte

// openAPISpec describes the API for tools that import it (ChatGPT Actions,
// Copilot extensions, any OpenAPI client).
//
//go:embed openapi.json
var openAPISpec []byte

// Asker answers a question. pipeline.Answerer implements it.
type Asker interface {
	Ask(ctx context.Context, question string) (pipeline.Answer, error)
}

// Searcher returns passages without calling a model. pipeline.Answerer
// implements it too; without it, /api/search answers 501.
type Searcher interface {
	Search(ctx context.Context, query string, k int) ([]domain.ScoredChunk, error)
}

// NewHandler returns the HTTP handler for the RAGscope API.
func NewHandler(asker Asker) http.Handler {
	s := &server{asker: asker}
	s.searcher, _ = asker.(Searcher)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /openapi.json", s.openapi)
	mux.HandleFunc("/api/ask", s.ask)
	mux.HandleFunc("/api/search", s.search)
	return mux
}

type server struct {
	asker    Asker
	searcher Searcher
}

const (
	defaultSearchK = 5
	maxSearchK     = 20
)

type searchRequest struct {
	Query string `json:"query"`
	K     int    `json:"k"`
}

type searchResponse struct {
	Sources []sourceJSON `json:"sources"`
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

func (s *server) openapi(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(openAPISpec)
}

// search returns the evidence only. The caller's own model writes the answer.
func (s *server) search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	if s.searcher == nil {
		writeError(w, http.StatusNotImplemented, "search is not available")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if utf8.RuneCountInString(query) > maxQuestionChars {
		writeError(w, http.StatusBadRequest, "query is too long (max 500 characters)")
		return
	}
	k := req.K
	if k <= 0 {
		k = defaultSearchK
	}
	k = min(k, maxSearchK)

	ctx, cancel := context.WithTimeout(r.Context(), askTimeout)
	defer cancel()

	found, err := s.searcher.Search(ctx, query, k)
	if err != nil {
		slog.Error("search failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not search")
		return
	}
	writeJSON(w, http.StatusOK, searchResponse{Sources: toSources(found)})
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

// toSources numbers the passages from 1, which matches the [1], [2] citations
// in an answer.
func toSources(chunks []domain.ScoredChunk) []sourceJSON {
	sources := make([]sourceJSON, 0, len(chunks))
	for i, s := range chunks {
		sources = append(sources, sourceJSON{
			N:       i + 1,
			ID:      s.ID,
			Article: s.Metadata["article"],
			Score:   s.Score,
			Text:    s.Text,
		})
	}
	return sources
}

func toResponse(a pipeline.Answer) askResponse {
	return askResponse{
		Answer:  a.Text,
		Refused: a.Refused,
		Sources: toSources(a.Sources),
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
