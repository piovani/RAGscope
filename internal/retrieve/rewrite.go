package retrieve

import (
	"context"
	"fmt"
	"strings"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/generate"
)

// Chatter is the small part of an LLM client that query rewriting needs.
// *generate.Ollama already implements it.
type Chatter interface {
	Chat(ctx context.Context, system, user string) (generate.Response, error)
}

// hydeSystem asks for a fake answer written like the law. The embedding of a
// passage that looks like an article is closer to the real articles than the
// embedding of a short question is. "No preamble" keeps chatter such as
// "Claro! Aqui está" out of the text we search with, and the word limit
// keeps the passage focused on one idea.
const hydeSystem = `Você escreve trechos hipotéticos da legislação brasileira de proteção de dados.
Dada uma pergunta, escreva UM trecho curto (no máximo 80 palavras), em português,
no estilo de um artigo de lei, que responderia a essa pergunta.
Responda apenas com o trecho: sem preâmbulo, sem título e sem explicação.`

// multiQuerySystem asks for rephrasings, one per line, so parsing is trivial.
// "Plain everyday language" is on purpose: people ask in their own words, the
// law uses formal ones, and a variety of wordings reaches more chunks. The ban
// on numbering and extra text keeps each line usable as a query.
const multiQuerySystem = `Você reescreve perguntas para um sistema de busca.
Dada uma pergunta, escreva %d formas diferentes de perguntar a mesma coisa,
com o mesmo significado, em português simples do dia a dia.
Escreva uma por linha, sem numeração e sem nenhum texto extra.`

// HyDE (Hypothetical Document Embeddings) searches with the question followed
// by a made-up passage that answers it. The passage may be wrong; it only needs
// to sound like the law so it lands near the right chunks.
type HyDE struct {
	Base Retriever
	LLM  Chatter
}

func (h HyDE) Name() string { return h.Base.Name() + "+hyde" }

func (h HyDE) Retrieve(ctx context.Context, query string, k int) ([]domain.ScoredChunk, error) {
	resp, err := h.LLM.Chat(ctx, hydeSystem, query)
	if err != nil {
		return nil, fmt.Errorf("hyde: write passage: %w", err)
	}
	passage := strings.TrimSpace(resp.Answer)
	if passage == "" {
		// No fallback to the plain question: the experiment must not hide this.
		return nil, fmt.Errorf("hyde: the model returned an empty passage")
	}
	return h.Base.Retrieve(ctx, query+"\n"+passage, k)
}

// defaultRephrasings is used when MultiQuery.N is not set.
const defaultRephrasings = 3

// MultiQuery searches with the question and with N rephrasings of it, then
// merges all the rankings with Reciprocal Rank Fusion. A chunk found by
// several wordings beats a chunk found by only one.
type MultiQuery struct {
	Base Retriever
	LLM  Chatter
	N    int
}

func (m MultiQuery) Name() string { return m.Base.Name() + "+multiquery" }

func (m MultiQuery) Retrieve(ctx context.Context, query string, k int) ([]domain.ScoredChunk, error) {
	n := m.N
	if n <= 0 {
		n = defaultRephrasings
	}

	resp, err := m.LLM.Chat(ctx, fmt.Sprintf(multiQuerySystem, n), query)
	if err != nil {
		return nil, fmt.Errorf("multiquery: rephrase: %w", err)
	}
	queries := append([]string{query}, parseRephrasings(resp.Answer, query, n)...)

	// Fusion works better when each search offers more than the final k.
	pool := max(2*k, 20)

	rankings := make([][]domain.ScoredChunk, 0, len(queries))
	for _, q := range queries {
		results, err := m.Base.Retrieve(ctx, q, pool)
		if err != nil {
			return nil, err
		}
		rankings = append(rankings, results)
	}
	return fuse(rankings, k), nil
}

// parseRephrasings turns the model text into clean queries. Models add bullets
// and numbers even when told not to, so we strip them, and we drop blank lines,
// repeated lines and copies of the original question. At most n are kept.
func parseRephrasings(text, original string, n int) []string {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(original)): true}
	var out []string

	for _, line := range strings.Split(text, "\n") {
		line = cleanLine(line)
		key := strings.ToLower(line)
		if line == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, line)
		if len(out) == n {
			break
		}
	}
	return out
}

// cleanLine removes a leading bullet ("-", "*", "•") or number ("1.", "2)").
func cleanLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimLeft(line, "-*• \t")

	digits := strings.TrimLeft(line, "0123456789")
	if digits != line && len(digits) > 0 && (digits[0] == '.' || digits[0] == ')') {
		line = digits[1:]
	}
	return strings.TrimSpace(line)
}
