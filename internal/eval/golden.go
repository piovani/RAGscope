// Package eval measures how good a RAG configuration is.
//
// The golden set does not point to chunk IDs, because those change with every
// chunking strategy. Instead each question carries literal snippets from the
// source text (the "evidence"). A chunk is relevant when it contains one of
// them, so the same golden set works for every experiment.
package eval

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/piovani/RAGscope/internal/domain"
)

// Question is one entry of the golden set.
type Question struct {
	ID       string `yaml:"id"`
	Question string `yaml:"question"`
	Answer   string `yaml:"answer,omitempty"` // reference answer, used later to judge generation

	// Evidence holds snippets copied word for word from the source documents.
	// Keep them short (one sentence or less) so a chunk border is unlikely to
	// cut them in half.
	Evidence []string `yaml:"evidence,omitempty"`

	// Unanswerable marks questions whose answer is NOT in the corpus. The system
	// should refuse to answer them. They have no evidence by definition.
	Unanswerable bool `yaml:"unanswerable,omitempty"`
}

// LoadQuestions reads and validates a golden set file.
func LoadQuestions(path string) ([]Question, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read golden set: %w", err)
	}

	var questions []Question
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true) // catch typos in field names
	if err := dec.Decode(&questions); err != nil {
		return nil, fmt.Errorf("parse golden set %s: %w", path, err)
	}
	if err := validate(questions); err != nil {
		return nil, fmt.Errorf("invalid golden set %s: %w", path, err)
	}
	return questions, nil
}

func validate(questions []Question) error {
	if len(questions) == 0 {
		return errors.New("no questions found")
	}

	var problems []error
	seen := map[string]bool{}

	for i, q := range questions {
		label := fmt.Sprintf("question #%d", i+1)
		if q.ID != "" {
			label = q.ID
		}

		switch {
		case q.ID == "":
			problems = append(problems, fmt.Errorf("%s: id is required", label))
		case seen[q.ID]:
			problems = append(problems, fmt.Errorf("%s: id is used twice", label))
		}
		seen[q.ID] = true

		if strings.TrimSpace(q.Question) == "" {
			problems = append(problems, fmt.Errorf("%s: question text is empty", label))
		}
		if q.Unanswerable && len(q.Evidence) > 0 {
			problems = append(problems, fmt.Errorf("%s: an unanswerable question cannot have evidence", label))
		}
		if !q.Unanswerable && len(q.Evidence) == 0 {
			problems = append(problems, fmt.Errorf("%s: evidence is required (or mark it unanswerable)", label))
		}
	}
	return errors.Join(problems...)
}

// MissingEvidence lists the evidence snippets that do not appear in any
// document. Such a question could never be found, and it would silently lower
// the scores, so it is better to catch it before running an experiment.
func MissingEvidence(questions []Question, docs []domain.Document) []string {
	var corpus []string
	for _, doc := range docs {
		corpus = append(corpus, normalize(doc.Text))
	}

	var missing []string
	for _, q := range questions {
		for _, snippet := range q.Evidence {
			if !appearsIn(snippet, corpus) {
				missing = append(missing, fmt.Sprintf("%s: %q", q.ID, snippet))
			}
		}
	}
	return missing
}

func appearsIn(snippet string, normalizedTexts []string) bool {
	want := normalize(snippet)
	for _, text := range normalizedTexts {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}

// normalize makes comparisons ignore line breaks, repeated spaces and case.
func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
