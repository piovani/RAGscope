package eval

import "testing"

func TestFingerprint(t *testing.T) {
	questions := []Question{
		{ID: "q1", Question: "O que é dado pessoal?", Evidence: []string{"informação relacionada"}},
		{ID: "q2", Question: "Qual o imposto?", Unanswerable: true},
	}

	same := Fingerprint(questions)
	if same != Fingerprint(append([]Question(nil), questions...)) {
		t.Error("the same questions must give the same fingerprint")
	}
	if len(same) != 12 {
		t.Errorf("fingerprint %q should have 12 characters", same)
	}

	changed := append([]Question(nil), questions...)
	changed[0].Question = "O que é um dado pessoal?"
	if Fingerprint(changed) == same {
		t.Error("changing a question must change the fingerprint")
	}
}
