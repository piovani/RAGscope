package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/pipeline"
)

type fakeSearcher struct {
	chunks []domain.ScoredChunk
	err    error
	gotK   int
}

func (f *fakeSearcher) Search(_ context.Context, _ string, k int) ([]domain.ScoredChunk, error) {
	f.gotK = k
	return f.chunks, f.err
}

type fakeAsker struct {
	answer pipeline.Answer
	err    error
}

func (f fakeAsker) Ask(context.Context, string) (pipeline.Answer, error) { return f.answer, f.err }

func chunk(article, text string) domain.ScoredChunk {
	return domain.ScoredChunk{Chunk: domain.Chunk{Text: text, Metadata: map[string]string{"article": article}}}
}

// run sends the lines to the server and returns the decoded responses.
func run(t *testing.T, s *Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("response is not JSON: %q", l)
		}
		got = append(got, m)
	}
	return got
}

func text(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	res := resp["result"].(map[string]any)
	c := res["content"].([]any)[0].(map[string]any)
	isErr, _ := res["isError"].(bool)
	return c["text"].(string), isErr
}

func TestInitializeEchoesVersionAndSkipsNotifications(t *testing.T) {
	s := &Server{Searcher: &fakeSearcher{}, Version: "test"}
	got := run(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
	)
	if len(got) != 2 {
		t.Fatalf("got %d responses, want 2 (a notification gets none)", len(got))
	}
	res := got[0]["result"].(map[string]any)
	if res["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v", res["protocolVersion"])
	}
	if res["serverInfo"].(map[string]any)["name"] != "ragscope" {
		t.Errorf("serverInfo = %v", res["serverInfo"])
	}
}

func TestToolsListDependsOnAsker(t *testing.T) {
	names := func(s *Server) []string {
		got := run(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		var out []string
		for _, tl := range got[0]["result"].(map[string]any)["tools"].([]any) {
			out = append(out, tl.(map[string]any)["name"].(string))
		}
		return out
	}
	if got := names(&Server{Searcher: &fakeSearcher{}}); len(got) != 1 || got[0] != "search_lgpd" {
		t.Errorf("without asker: %v", got)
	}
	if got := names(&Server{Searcher: &fakeSearcher{}, Asker: fakeAsker{}}); len(got) != 2 {
		t.Errorf("with asker: %v", got)
	}
}

func TestSearchTool(t *testing.T) {
	f := &fakeSearcher{chunks: []domain.ScoredChunk{chunk("5", "dado pessoal é..."), chunk("", "outro")}}
	got := run(t, &Server{Searcher: f},
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_lgpd","arguments":{"query":"dado pessoal","k":999}}}`)

	body, isErr := text(t, got[0])
	if isErr {
		t.Fatalf("unexpected error: %s", body)
	}
	if !strings.Contains(body, "[1] Art. 5\ndado pessoal é...") || !strings.Contains(body, "[2]\noutro") {
		t.Errorf("body = %q", body)
	}
	if f.gotK != maxK {
		t.Errorf("k = %d, want it clamped to %d", f.gotK, maxK)
	}
}

func TestToolErrorsAreResultsNotProtocolErrors(t *testing.T) {
	s := &Server{Searcher: &fakeSearcher{err: errors.New("pg password=secret")}}
	got := run(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_lgpd","arguments":{"query":"q"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_lgpd","arguments":{"query":"  "}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ask_lgpd","arguments":{"question":"q"}}}`,
	)
	for i, r := range got {
		body, isErr := text(t, r)
		if !isErr {
			t.Errorf("response %d should be an error result: %s", i, body)
		}
		if strings.Contains(body, "secret") {
			t.Errorf("response %d leaks the cause: %s", i, body)
		}
	}
}

func TestAskTool(t *testing.T) {
	s := &Server{Searcher: &fakeSearcher{}, Asker: fakeAsker{answer: pipeline.Answer{
		Text: "Em 15 dias [1].", Sources: []domain.ScoredChunk{chunk("19", "texto")},
	}}}
	got := run(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ask_lgpd","arguments":{"question":"prazo?"}}}`)
	body, isErr := text(t, got[0])
	if isErr || !strings.HasPrefix(body, "Em 15 dias [1].") || !strings.Contains(body, "[1] Art. 19") {
		t.Errorf("body = %q (error=%v)", body, isErr)
	}

	s.Asker = fakeAsker{answer: pipeline.Answer{Refused: true}}
	got = run(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ask_lgpd","arguments":{"question":"clima?"}}}`)
	if body, _ := text(t, got[0]); !strings.Contains(body, "do not contain the answer") {
		t.Errorf("refusal body = %q", body)
	}
}

func TestUnknownMethodAndBadJSON(t *testing.T) {
	got := run(t, &Server{Searcher: &fakeSearcher{}},
		`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`,
		`not json`,
	)
	if code := got[0]["error"].(map[string]any)["code"].(float64); code != -32601 {
		t.Errorf("code = %v, want -32601", code)
	}
	if code := got[1]["error"].(map[string]any)["code"].(float64); code != -32700 {
		t.Errorf("code = %v, want -32700", code)
	}
}
