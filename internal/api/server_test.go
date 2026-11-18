package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/generate"
	"github.com/piovani/RAGscope/internal/pipeline"
)

type fakeAsker struct {
	answer pipeline.Answer
	err    error
	got    string
}

func (f *fakeAsker) Ask(_ context.Context, q string) (pipeline.Answer, error) {
	f.got = q
	return f.answer, f.err
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(body))
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	NewHandler(&fakeAsker{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Errorf("body = %q", got)
	}
}

func TestHealthzRejectsPost(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	NewHandler(&fakeAsker{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestIndexPage(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	NewHandler(&fakeAsker{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "<form") {
		t.Error("page has no form")
	}
}

func TestAskSuccess(t *testing.T) {
	t.Parallel()

	asker := &fakeAsker{answer: pipeline.Answer{
		Text: "Em 15 dias [1].",
		Sources: []domain.ScoredChunk{
			{Chunk: domain.Chunk{ID: "lgpd-0006", Text: "texto", Metadata: map[string]string{"article": "5"}}, Score: 0.67},
			{Chunk: domain.Chunk{ID: "lgpd-0007", Text: "outro"}, Score: 0.5},
		},
		Usage: generate.Usage{InputTokens: 10, OutputTokens: 5, Latency: 1500 * time.Millisecond},
	}}

	rec := post(NewHandler(asker), `{"question": "  quanto tempo?  "}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if asker.got != "quanto tempo?" {
		t.Errorf("asker got %q, want trimmed question", asker.got)
	}
	var got askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Answer != "Em 15 dias [1]." || got.Refused || len(got.Sources) != 2 {
		t.Fatalf("got %+v", got)
	}
	if s := got.Sources[0]; s.N != 1 || s.ID != "lgpd-0006" || s.Article != "5" || s.Score != 0.67 {
		t.Errorf("source 0 = %+v", s)
	}
	if s := got.Sources[1]; s.N != 2 || s.Article != "" {
		t.Errorf("source 1 = %+v", s)
	}
	if got.Usage.InputTokens != 10 || got.Usage.LatencyMS != 1500 {
		t.Errorf("usage = %+v", got.Usage)
	}
}

func TestAskRefusal(t *testing.T) {
	t.Parallel()

	rec := post(NewHandler(&fakeAsker{answer: pipeline.Answer{Refused: true}}), `{"question":"q"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Refused {
		t.Error("refused = false")
	}
	if !strings.Contains(rec.Body.String(), `"sources":[]`) {
		t.Errorf("sources should be an empty list, body %s", rec.Body)
	}
}

func TestAskBadInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{"malformed JSON", `{"question": `},
		{"empty question", `{"question": "   "}`},
		{"missing question", `{}`},
		{"too long", `{"question": "` + strings.Repeat("a", 501) + `"}`},
		{"body too big", `{"question": "` + strings.Repeat("a", 5000) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			asker := &fakeAsker{}
			rec := post(NewHandler(asker), tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("body = %s", rec.Body)
			}
			if asker.got != "" {
				t.Error("asker must not be called")
			}
		})
	}
}

func TestAskWrongMethod(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	NewHandler(&fakeAsker{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ask", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestAskerErrorIsNotLeaked(t *testing.T) {
	t.Parallel()

	asker := &fakeAsker{err: errors.New("postgres: password for host 10.0.0.5 failed")}
	rec := post(NewHandler(asker), `{"question":"q"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got := rec.Body.String(); got != "{\"error\":\"could not answer the question\"}\n" {
		t.Errorf("body = %q", got)
	}
}

type searchAsker struct {
	fakeAsker
	chunks []domain.ScoredChunk
	err    error
	gotQ   string
	gotK   int
}

func (f *searchAsker) Search(_ context.Context, q string, k int) ([]domain.ScoredChunk, error) {
	f.gotQ, f.gotK = q, k
	return f.chunks, f.err
}

func postTo(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func TestSearchReturnsNumberedPassages(t *testing.T) {
	t.Parallel()

	f := &searchAsker{chunks: []domain.ScoredChunk{
		{Chunk: domain.Chunk{ID: "a", Text: "texto A", Metadata: map[string]string{"article": "5"}}, Score: 0.9},
		{Chunk: domain.Chunk{ID: "b", Text: "texto B"}, Score: 0.4},
	}}
	rec := postTo(NewHandler(f), "/api/search", `{"query":" dado pessoal ","k":2}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 2 || got.Sources[0].N != 1 || got.Sources[1].N != 2 || got.Sources[0].Article != "5" {
		t.Errorf("sources = %+v", got.Sources)
	}
	if f.gotQ != "dado pessoal" || f.gotK != 2 {
		t.Errorf("searcher got %q, %d", f.gotQ, f.gotK)
	}
}

func TestSearchClampsK(t *testing.T) {
	t.Parallel()

	for body, want := range map[string]int{
		`{"query":"q"}`:          defaultSearchK,
		`{"query":"q","k":-3}`:   defaultSearchK,
		`{"query":"q","k":9999}`: maxSearchK,
		`{"query":"q","k":7}`:    7,
	} {
		f := &searchAsker{}
		if rec := postTo(NewHandler(f), "/api/search", body); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", body, rec.Code)
		}
		if f.gotK != want {
			t.Errorf("%s: k = %d, want %d", body, f.gotK, want)
		}
	}
}

func TestSearchRejectsBadInput(t *testing.T) {
	t.Parallel()

	long := `{"query":"` + strings.Repeat("a", maxQuestionChars+1) + `"}`
	for name, body := range map[string]string{"empty": `{"query":"  "}`, "bad json": `{`, "too long": long} {
		rec := postTo(NewHandler(&searchAsker{}), "/api/search", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
}

func TestSearchErrorHidesCause(t *testing.T) {
	t.Parallel()

	rec := postTo(NewHandler(&searchAsker{err: errors.New("pg password=secret")}), "/api/search", `{"query":"q"}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestSearchNotImplementedWithoutSearcher(t *testing.T) {
	t.Parallel()

	rec := postTo(NewHandler(&fakeAsker{}), "/api/search", `{"query":"q"}`)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestSearchRejectsGet(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	NewHandler(&searchAsker{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestOpenAPISpecIsValidAndCoversTheRoutes(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	NewHandler(&fakeAsker{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var spec struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("spec is not JSON: %v", err)
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		t.Errorf("openapi = %q", spec.OpenAPI)
	}
	for _, path := range []string{"/api/search", "/api/ask", "/healthz"} {
		if _, ok := spec.Paths[path]; !ok {
			t.Errorf("spec is missing %s", path)
		}
	}
}
