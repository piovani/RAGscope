package eval

import (
	"context"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/generate"
)

// scriptedGenerator answers from a fixed table, keyed by the question text.
type scriptedGenerator map[string]generate.Response

func (scriptedGenerator) Name() string { return "scripted" }
func (g scriptedGenerator) Generate(_ context.Context, req generate.Request) (generate.Response, error) {
	return g[req.Question], nil
}

type fixedPassages struct{}

func (fixedPassages) Name() string { return "fixed" }
func (fixedPassages) Retrieve(context.Context, string, int) ([]domain.ScoredChunk, error) {
	return []domain.ScoredChunk{{Chunk: domain.Chunk{ID: "c1", Text: "texto"}}}, nil
}

// judgeByWord says "supported" unless the answer contains "inventado".
type judgeByWord struct{}

func (judgeByWord) Supported(_ context.Context, _ string, _ []domain.ScoredChunk, answer string) (bool, error) {
	return !contains(answer, "inventado"), nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRunAnswersScoresRefusalsCitationsAndFaithfulness(t *testing.T) {
	questions := []Question{
		{ID: "a1", Question: "responde bem"},
		{ID: "a2", Question: "responde sem citar"},
		{ID: "a3", Question: "recusa por engano"},
		{ID: "a4", Question: "inventa"},
		{ID: "u1", Question: "recusa certo", Unanswerable: true},
		{ID: "u2", Question: "responde errado", Unanswerable: true},
	}
	generator := scriptedGenerator{
		"responde bem":       {Answer: "O prazo é de 15 dias [1]."},
		"responde sem citar": {Answer: "O prazo é de 15 dias."},
		"recusa por engano":  {Answer: "SEM_RESPOSTA", Refused: true},
		"inventa":            {Answer: "Algo inventado [1]."},
		"recusa certo":       {Answer: "SEM_RESPOSTA", Refused: true},
		"responde errado":    {Answer: "A alíquota é 27%."},
	}

	report, err := RunAnswers(context.Background(), fixedPassages{}, generator, judgeByWord{}, questions, 5)
	if err != nil {
		t.Fatal(err)
	}

	if report.Answerable != 4 || report.Unanswerable != 2 {
		t.Errorf("answerable=%d unanswerable=%d, want 4 and 2", report.Answerable, report.Unanswerable)
	}
	assertClose(t, "correct refusal", report.CorrectRefusal, 0.5) // u1 yes, u2 no
	assertClose(t, "false refusal", report.FalseRefusal, 0.25)    // a3 of 4
	// Real answers: a1, a2, a4, u2. Those with a [n] citation: a1, a4.
	assertClose(t, "citation", report.Citation, 0.5)
	// Judged (not refused): a1, a2, a4, u2. Only a4 is "inventado".
	if report.Judged != 4 {
		t.Errorf("judged = %d, want 4", report.Judged)
	}
	assertClose(t, "faithfulness", report.Faithfulness, 0.75)
}

func TestRunAnswersWithoutJudge(t *testing.T) {
	questions := []Question{{ID: "a1", Question: "q"}}
	report, err := RunAnswers(context.Background(), fixedPassages{}, scriptedGenerator{"q": {Answer: "ok"}}, nil, questions, 5)
	if err != nil {
		t.Fatal(err)
	}
	if report.Judged != 0 || report.Results[0].Supported != nil {
		t.Error("without a judge nothing should be judged")
	}
}

func TestParseVerdict(t *testing.T) {
	tests := []struct {
		answer  string
		want    bool
		wantErr bool
	}{
		{"SUPORTADA", true, false},
		{"suportada.", true, false},
		{"NAO_SUPORTADA", false, false}, // contains SUPORTADA, but means the opposite
		{"Resposta: NAO_SUPORTADA, pois...", false, false},
		{"talvez", false, true},
	}
	for _, tt := range tests {
		got, err := parseVerdict(tt.answer)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("parseVerdict(%q) = %v, %v; want %v (error: %v)", tt.answer, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestSampleSpreadsQuestionsEvenly(t *testing.T) {
	var questions []Question
	for i := range 10 {
		questions = append(questions, Question{ID: string(rune('a' + i))})
	}

	got := Sample(questions, 5)
	if len(got) != 5 || got[0].ID != "a" || got[1].ID != "c" || got[4].ID != "i" {
		t.Errorf("sample = %v", got)
	}
	if len(Sample(questions, 0)) != 10 || len(Sample(questions, 99)) != 10 {
		t.Error("asking for 0 or more than there are should return everything")
	}
}
