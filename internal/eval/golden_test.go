package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piovani/RAGscope/internal/domain"
)

func writeGolden(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "questions.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadQuestions(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string // empty means it should load fine
	}{
		{
			name: "answerable and unanswerable",
			yaml: `
- id: q1
  question: What is personal data?
  evidence: ["information related to an identified person"]
- id: q2
  question: What is the income tax rate?
  unanswerable: true
`,
		},
		{name: "empty file", yaml: "[]", wantErr: "no questions"},
		{
			name:    "answerable without evidence",
			yaml:    "- {id: q1, question: hello}",
			wantErr: "evidence is required",
		},
		{
			name:    "unanswerable with evidence",
			yaml:    "- {id: q1, question: hello, unanswerable: true, evidence: [x]}",
			wantErr: "cannot have evidence",
		},
		{
			name:    "repeated id",
			yaml:    "- {id: q1, question: a, evidence: [x]}\n- {id: q1, question: b, evidence: [y]}",
			wantErr: "used twice",
		},
		{
			name:    "typo in a field name",
			yaml:    "- {id: q1, question: a, evidnce: [x]}",
			wantErr: "evidnce",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadQuestions(writeGolden(t, tt.yaml))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestMissingEvidence(t *testing.T) {
	docs := []domain.Document{{ID: "lei", Text: "Art. 5º  Para os fins\ndesta Lei, considera-se: dado pessoal."}}
	questions := []Question{
		{ID: "ok", Evidence: []string{"para os fins desta lei"}}, // different case, spaces and line break
		{ID: "bad", Evidence: []string{"texto que nao existe"}},
	}

	missing := MissingEvidence(questions, docs)

	if len(missing) != 1 || !strings.HasPrefix(missing[0], "bad:") {
		t.Errorf("missing = %v, want only the \"bad\" question", missing)
	}
}
