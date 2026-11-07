package eval

import (
	"context"
	"fmt"
	"strings"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/generate"
)

// Chatter is a model we can send a system and a user message to.
type Chatter interface {
	Chat(ctx context.Context, system, user string) (generate.Response, error)
}

const judgePrompt = `Você é um avaliador rigoroso. Você recebe uma pergunta, trechos de uma lei e uma resposta.
Decida se TODAS as afirmações da resposta estão sustentadas pelos trechos.

Responda com UMA única palavra:
- SUPORTADA, se tudo o que a resposta afirma aparece nos trechos;
- NAO_SUPORTADA, se a resposta afirma algo que os trechos não dizem ou que os contradiz.`

const correctnessPrompt = `Você é um avaliador rigoroso. Você recebe uma pergunta, a RESPOSTA DE REFERÊNCIA (que está correta) e uma RESPOSTA a avaliar.
Decida se a resposta diz o mesmo que a referência nos pontos principais.

Responda com UMA única palavra:
- CORRETA, se a resposta traz o ponto principal da referência e não a contradiz. Detalhes a mais que não contradizem a referência não a tornam incorreta;
- INCORRETA, se a resposta contradiz a referência, responde outra coisa ou deixa de fora o ponto principal.`

// LLMJudge asks a language model to grade answers.
//
// This is a first, simple version. A small model is a weak judge, so its scores
// must be checked against human grades before they are trusted.
type LLMJudge struct {
	Model Chatter
}

func (j LLMJudge) Supported(ctx context.Context, question string, passages []domain.ScoredChunk, answer string) (bool, error) {
	var b strings.Builder
	b.WriteString("Trechos:\n\n")
	for i, p := range passages {
		fmt.Fprintf(&b, "[%d] %s\n\n", i+1, p.Text)
	}
	fmt.Fprintf(&b, "Pergunta: %s\n\nResposta: %s", question, answer)

	resp, err := j.Model.Chat(ctx, judgePrompt, b.String())
	if err != nil {
		return false, err
	}
	return parseVerdict(resp.Answer)
}

// Correct compares the answer with the reference answer. It does not send the
// passages, so it is much shorter and faster than the faithfulness check.
func (j LLMJudge) Correct(ctx context.Context, question, reference, answer string) (bool, error) {
	user := fmt.Sprintf("Pergunta: %s\n\nResposta de referência: %s\n\nResposta a avaliar: %s", question, reference, answer)

	resp, err := j.Model.Chat(ctx, correctnessPrompt, user)
	if err != nil {
		return false, err
	}
	return parseCorrectness(resp.Answer)
}

// parseCorrectness reads the one-word verdict. INCORRETA contains the letters of
// CORRETA, so it has to be checked first.
func parseCorrectness(answer string) (bool, error) {
	upper := strings.ToUpper(answer)
	switch {
	case strings.Contains(upper, "INCORRETA"):
		return false, nil
	case strings.Contains(upper, "CORRETA"):
		return true, nil
	}
	return false, fmt.Errorf("the judge did not answer CORRETA or INCORRETA: %.80q", answer)
}

// parseVerdict reads the judge's one-word answer. NAO_SUPORTADA contains the
// word SUPORTADA, so it has to be checked first.
func parseVerdict(answer string) (bool, error) {
	upper := strings.ToUpper(answer)
	switch {
	case strings.Contains(upper, "NAO_SUPORTADA") || strings.Contains(upper, "NÃO_SUPORTADA"):
		return false, nil
	case strings.Contains(upper, "SUPORTADA"):
		return true, nil
	}
	return false, fmt.Errorf("the judge did not answer SUPORTADA or NAO_SUPORTADA: %.80q", answer)
}
