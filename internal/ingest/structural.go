package ingest

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/piovani/RAGscope/internal/domain"
)

// An article starts on a line that begins with a capital "Art." and its number,
// like "Art. 5º" or "Art. 12-A". Cross references ("conforme o art. 5º") are
// written in lowercase, so they are ignored even when a line break puts them at
// the start of a line.
var articleStart = regexp.MustCompile(`(?m)^[ \t]*(?:Art|ART)\.?[ \t]*(\d+(?:-[A-Z])?)`)

// Structural makes one chunk per article, which keeps a legal rule whole
// instead of cutting it in the middle. Articles longer than MaxWords are split
// with the same sliding window as Fixed. A text without articles falls back
// to that window as well.
type Structural struct {
	MaxWords int
	Overlap  int
}

func (s Structural) Name() string { return "structural" }

func (s Structural) Chunk(doc domain.Document) ([]domain.Chunk, error) {
	if s.MaxWords <= 0 {
		return nil, errors.New("max words must be greater than zero")
	}

	var chunks []domain.Chunk
	for _, piece := range splitArticles(doc.Text) {
		texts, err := s.fit(piece.text)
		if err != nil {
			return nil, err
		}

		for _, text := range texts {
			metadata := map[string]string{"title": doc.Title}
			if piece.article != "" {
				metadata["article"] = piece.article
			}

			chunks = append(chunks, domain.Chunk{
				ID:         fmt.Sprintf("%s-%04d", doc.ID, len(chunks)),
				DocumentID: doc.ID,
				Index:      len(chunks),
				Text:       text,
				Metadata:   metadata,
			})
		}
	}
	return chunks, nil
}

// fit returns the text as one piece, or split in windows when it is too long.
func (s Structural) fit(text string) ([]string, error) {
	words := strings.Fields(text)
	if len(words) <= s.MaxWords {
		return []string{strings.Join(words, " ")}, nil
	}

	windows, err := Fixed{Size: s.MaxWords, Overlap: s.Overlap}.Chunk(domain.Document{Text: text})
	if err != nil {
		return nil, err
	}

	texts := make([]string, len(windows))
	for i, w := range windows {
		texts[i] = w.Text
	}
	return texts, nil
}

type piece struct {
	article string // "5", "12-A"... empty for text before the first article
	text    string
}

// splitArticles cuts the text at every line that starts an article.
func splitArticles(text string) []piece {
	starts := articleStart.FindAllStringSubmatchIndex(text, -1)
	if len(starts) == 0 {
		return nonEmpty([]piece{{text: text}})
	}

	// Whatever comes before the first article (title, summary) is kept apart.
	pieces := []piece{{text: text[:starts[0][0]]}}

	for i, match := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		pieces = append(pieces, piece{
			article: strings.ToUpper(text[match[2]:match[3]]),
			text:    text[match[0]:end],
		})
	}
	return nonEmpty(pieces)
}

func nonEmpty(pieces []piece) []piece {
	var kept []piece
	for _, p := range pieces {
		if strings.TrimSpace(p.text) != "" {
			kept = append(kept, p)
		}
	}
	return kept
}
