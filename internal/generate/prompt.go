package generate

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// RefusalMarker is what the model must answer when the retrieved passages do
// not contain the answer. A fixed word is easy and reliable to detect, unlike
// a polite sentence that can be worded in many ways.
const RefusalMarker = "SEM_RESPOSTA"

const systemPrompt = `Você responde perguntas sobre a LGPD (Lei Geral de Proteção de Dados) usando SOMENTE os trechos da lei fornecidos.

Regras:
- Use apenas as informações dos trechos. Não use conhecimento externo.
- Cite a fonte de cada afirmação com o número do trecho entre colchetes, como [1] ou [2].
- Se os trechos não trouxerem a resposta, responda exatamente: ` + RefusalMarker + `
- Responda em português, de forma direta e curta.`

// buildPrompt turns the question and the retrieved passages into the messages
// sent to the model. Passages are numbered, so the answer can cite them.
func buildPrompt(req Request) (system, user string) {
	var b strings.Builder
	b.WriteString("Trechos da lei:\n\n")
	for i, chunk := range req.Context {
		fmt.Fprintf(&b, "[%d] %s\n%s\n\n", i+1, Label(chunk.Chunk.Metadata["article"], chunk.ID), chunk.Text)
	}
	fmt.Fprintf(&b, "Pergunta: %s", req.Question)
	return systemPrompt, b.String()
}

// Label names a passage for people: "Art. 5" when the chunk knows its article,
// and the chunk id (which always exists) otherwise.
func Label(article, id string) string {
	if article != "" {
		return fmt.Sprintf("(Art. %s, trecho %s)", article, id)
	}
	return fmt.Sprintf("(trecho %s)", id)
}

// isRefusal tells whether the model declined to answer. Models sometimes add a
// full stop or a short comment around the marker, so we accept the marker in a
// short answer. A long answer that merely mentions the word is a real answer.
func isRefusal(answer string) bool {
	return utf8.RuneCountInString(answer) <= 120 &&
		strings.Contains(strings.ToUpper(answer), RefusalMarker)
}
