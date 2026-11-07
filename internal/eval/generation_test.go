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

// Correct says "wrong" for answers that contain "errado".
func (judgeByWord) Correct(_ context.Context, _, _, answer string) (bool, error) {
	return !contains(answer, "errado"), nil
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

	report, err := RunAnswers(context.Background(), fixedPassages{}, generator, judgeByWord{}, JudgeOptions{Faithfulness: true}, questions, 5)
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
	report, err := RunAnswers(context.Background(), fixedPassages{}, scriptedGenerator{"q": {Answer: "ok"}}, nil, JudgeOptions{}, questions, 5)
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

func TestRunAnswersSplitsFalseRefusalsByCause(t *testing.T) {
	// fixedPassages always returns one passage with the text "texto".
	questions := []Question{
		{ID: "a1", Question: "q1", Evidence: []string{"texto"}},          // evidence is there, model refuses
		{ID: "a2", Question: "q2", Evidence: []string{"nunca aparece"}},  // evidence is missing, model refuses
		{ID: "a3", Question: "q3", Evidence: []string{"texto"}},          // answered
		{ID: "a4", Question: "q4", Evidence: []string{"texto", "falta"}}, // only part of the evidence is there
	}
	refusal := generate.Response{Answer: "SEM_RESPOSTA", Refused: true}
	generator := scriptedGenerator{
		"q1": refusal,
		"q2": refusal,
		"q3": {Answer: "ok"},
		"q4": refusal,
	}

	report, err := RunAnswers(context.Background(), fixedPassages{}, generator, nil, JudgeOptions{}, questions, 5)
	if err != nil {
		t.Fatal(err)
	}

	// Only a1 had the whole evidence in front of it. a2 and a4 are the retriever's fault.
	assertClose(t, "refused with evidence", report.FalseRefusalWithEvidence, 0.25)
	assertClose(t, "refused without evidence", report.FalseRefusalNoEvidence, 0.5)
	assertClose(t, "total false refusal", report.FalseRefusal, 0.75)

	if got := report.Results[0].EvidenceRetrieved; got == nil || !*got {
		t.Error("a1 should be marked as having the evidence")
	}
	if got := report.Results[3].EvidenceRetrieved; got == nil || *got {
		t.Error("a4 has only part of the evidence, so it should be marked as missing it")
	}
	if got := report.Results[0].Passages; len(got) != 1 || got[0] != "texto" {
		t.Errorf("passages = %v, want the text the model saw", got)
	}
}

func TestRunAnswersCorrectness(t *testing.T) {
	questions := []Question{
		{ID: "a1", Question: "q1", Answer: "referência 1"},
		{ID: "a2", Question: "q2", Answer: "referência 2"},
		{ID: "a3", Question: "q3", Answer: "referência 3"}, // refused: counts as wrong without being judged
		{ID: "u1", Question: "q4", Unanswerable: true},     // answered, but there was nothing to answer
	}
	generator := scriptedGenerator{
		"q1": {Answer: "certo"},
		"q2": {Answer: "isto está errado"},
		"q3": {Answer: "SEM_RESPOSTA", Refused: true},
		"q4": {Answer: "inventou algo"},
	}

	report, err := RunAnswers(context.Background(), fixedPassages{}, generator, judgeByWord{}, JudgeOptions{Correctness: true}, questions, 5)
	if err != nil {
		t.Fatal(err)
	}

	// Only a1 and a2 are compared with a reference: a3 was refused and u1 has none.
	if report.CorrectJudged != 2 {
		t.Errorf("judged = %d, want 2", report.CorrectJudged)
	}
	// 1 correct out of the 3 answerable questions; the refusal counts as wrong.
	assertClose(t, "correct", report.Correct, 1.0/3)
	if report.Judged != 0 {
		t.Error("faithfulness was not requested, so nothing should be judged for it")
	}
	if report.Results[2].Correct != nil || report.Results[3].Correct != nil {
		t.Error("a refusal and an answer to an unanswerable question must not be judged")
	}
}

func TestRunAnswersNeedsAReferenceAnswerToJudgeCorrectness(t *testing.T) {
	questions := []Question{{ID: "a1", Question: "q1"}} // no reference answer

	_, err := RunAnswers(context.Background(), fixedPassages{}, scriptedGenerator{"q1": {Answer: "ok"}}, judgeByWord{}, JudgeOptions{Correctness: true}, questions, 5)
	if err == nil {
		t.Fatal("expected an error: there is nothing to compare the answer with")
	}
}

func TestRunAnswersSumsTheCost(t *testing.T) {
	questions := []Question{{ID: "a1", Question: "q1"}, {ID: "a2", Question: "q2"}}
	generator := scriptedGenerator{
		"q1": {Answer: "ok", Usage: generate.Usage{CostUSD: 0.0125}},
		"q2": {Answer: "ok", Usage: generate.Usage{CostUSD: 0.0075}},
	}

	report, err := RunAnswers(context.Background(), fixedPassages{}, generator, nil, JudgeOptions{}, questions, 5)
	if err != nil {
		t.Fatal(err)
	}
	assertClose(t, "cost", report.CostUSD, 0.02)
}

func TestParseCorrectness(t *testing.T) {
	tests := []struct {
		answer  string
		want    bool
		wantErr bool
	}{
		{"CORRETA", true, false},
		{"correta.", true, false},
		{"INCORRETA", false, false}, // contains CORRETA, but means the opposite
		{"Resposta: INCORRETA, pois...", false, false},
		{"mais ou menos", false, true},
	}
	for _, tt := range tests {
		got, err := parseCorrectness(tt.answer)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("parseCorrectness(%q) = %v, %v; want %v (error: %v)", tt.answer, got, err, tt.want, tt.wantErr)
		}
	}
}
