// Package mcp is a small Model Context Protocol server over stdio. MCP is an
// open protocol that Claude, GitHub Copilot (VS Code), Cursor, ChatGPT's desktop
// and developer tools, and many other assistants speak, so one server makes
// RAGscope usable from all of them with no vendor SDK.
//
// Only what RAGscope needs is implemented: initialize, ping, tools/list and
// tools/call, with messages as one JSON object per line.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/pipeline"
)

const (
	defaultProtocol = "2025-06-18"
	maxLineBytes    = 1 << 20
	maxQueryChars   = 500
	defaultK        = 5
	maxK            = 20
)

// Searcher returns passages without calling a model.
type Searcher interface {
	Search(ctx context.Context, query string, k int) ([]domain.ScoredChunk, error)
}

// Asker answers a question with the configured model. It may be nil, and then
// only the search tool is offered (the caller's own model does the answering).
type Asker interface {
	Ask(ctx context.Context, question string) (pipeline.Answer, error)
}

// Server serves the tools.
type Server struct {
	Searcher Searcher
	Asker    Asker // optional
	Version  string
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests from in and writes responses to out until in closes or
// ctx ends. Only protocol messages go to out; logs must go elsewhere.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if resp := s.handle(ctx, []byte(line)); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return fmt.Errorf("write response: %w", err)
			}
		}
	}
	return scanner.Err()
}

// handle returns nil for notifications, which get no answer.
func (s *Server) handle(ctx context.Context, line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "parse error"}}
	}
	if len(req.ID) == 0 { // a notification, e.g. notifications/initialized
		return nil
	}

	reply := func(result any) *response { return &response{JSONRPC: "2.0", ID: req.ID, Result: result} }
	fail := func(code int, msg string) *response {
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: code, Message: msg}}
	}

	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := p.ProtocolVersion
		if version == "" {
			version = defaultProtocol
		}
		return reply(map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "ragscope", "version": s.Version},
		})
	case "ping":
		return reply(map[string]any{})
	case "tools/list":
		return reply(map[string]any{"tools": s.tools()})
	case "tools/call":
		return reply(s.call(ctx, req.Params))
	}
	return fail(-32601, "method not found: "+req.Method)
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (s *Server) tools() []tool {
	tools := []tool{{
		Name: "search_lgpd",
		Description: "Find the passages of the Brazilian data protection law (LGPD, Lei 13.709/2018) that best match a query. " +
			"Returns the literal text of each passage, numbered from 1, best first. No model is called: use these passages to write the answer yourself and cite them as [n].",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "What to look for, in Portuguese."},
				"k":     map[string]any{"type": "integer", "minimum": 1, "maximum": maxK, "default": defaultK, "description": "How many passages to return."},
			},
		},
	}}
	if s.Asker != nil {
		tools = append(tools, tool{
			Name: "ask_lgpd",
			Description: "Answer a question about the LGPD using only the law, with cited passages. " +
				"The answer is refused when the law does not cover the question.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"question"},
				"properties": map[string]any{
					"question": map[string]any{"type": "string", "description": "The question, in Portuguese."},
				},
			},
		})
	}
	return tools
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

func textResult(text string) toolResult {
	return toolResult{Content: []content{{Type: "text", Text: text}}}
}

func errorResult(msg string) toolResult {
	r := textResult(msg)
	r.IsError = true
	return r
}

func (s *Server) call(ctx context.Context, raw json.RawMessage) toolResult {
	var p struct {
		Name      string `json:"name"`
		Arguments struct {
			Query    string `json:"query"`
			Question string `json:"question"`
			K        int    `json:"k"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return errorResult("invalid arguments")
	}

	switch p.Name {
	case "search_lgpd":
		return s.search(ctx, p.Arguments.Query, p.Arguments.K)
	case "ask_lgpd":
		if s.Asker == nil {
			return errorResult("ask_lgpd is not enabled on this server")
		}
		return s.ask(ctx, p.Arguments.Question)
	}
	return errorResult("unknown tool: " + p.Name)
}

func validText(text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "the text is required"
	}
	if len([]rune(text)) > maxQueryChars {
		return "", fmt.Sprintf("the text is too long (max %d characters)", maxQueryChars)
	}
	return text, ""
}

func (s *Server) search(ctx context.Context, query string, k int) toolResult {
	query, problem := validText(query)
	if problem != "" {
		return errorResult(problem)
	}
	if k <= 0 {
		k = defaultK
	}
	k = min(k, maxK)

	found, err := s.Searcher.Search(ctx, query, k)
	if err != nil {
		// The cause stays in the log; the assistant gets nothing to dig into.
		slog.Error("mcp search failed", "err", err)
		return errorResult("could not search")
	}
	if len(found) == 0 {
		return textResult("No passages found.")
	}
	return textResult(formatSources(found))
}

func (s *Server) ask(ctx context.Context, question string) toolResult {
	question, problem := validText(question)
	if problem != "" {
		return errorResult(problem)
	}
	answer, err := s.Asker.Ask(ctx, question)
	if err != nil {
		slog.Error("mcp ask failed", "err", err)
		return errorResult("could not answer the question")
	}
	if answer.Refused {
		return textResult("The law passages do not contain the answer to this question.\n\n" + formatSources(answer.Sources))
	}
	return textResult(answer.Text + "\n\nSources:\n" + formatSources(answer.Sources))
}

func formatSources(chunks []domain.ScoredChunk) string {
	var b strings.Builder
	for i, c := range chunks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "[%d]", i+1)
		if article := c.Metadata["article"]; article != "" {
			fmt.Fprintf(&b, " Art. %s", article)
		}
		b.WriteString("\n")
		b.WriteString(c.Text)
	}
	return b.String()
}
