package ingest

import (
	"errors"
	"fmt"
	"strings"

	"github.com/piovani/RAGscope/internal/domain"
)

// Fixed cuts a document into windows of the same size, with some overlap so a
// sentence on the border is not lost.
//
// Size and Overlap count words, not model tokens. It is a rough measure, but
// it needs no tokenizer and is the same for every embedding model.
type Fixed struct {
	Size    int
	Overlap int
}

func (f Fixed) Name() string { return "fixed" }

func (f Fixed) Chunk(doc domain.Document) ([]domain.Chunk, error) {
	if f.Size <= 0 {
		return nil, errors.New("chunk size must be greater than zero")
	}
	if f.Overlap < 0 || f.Overlap >= f.Size {
		return nil, errors.New("overlap must be at least zero and smaller than the chunk size")
	}

	words := strings.Fields(doc.Text)
	step := f.Size - f.Overlap

	var chunks []domain.Chunk
	for start := 0; start < len(words); start += step {
		end := min(start+f.Size, len(words))

		chunks = append(chunks, domain.Chunk{
			ID:         fmt.Sprintf("%s-%04d", doc.ID, len(chunks)),
			DocumentID: doc.ID,
			Index:      len(chunks),
			Text:       strings.Join(words[start:end], " "),
			Metadata:   map[string]string{"title": doc.Title},
		})

		// The last window already reached the end; another one would only
		// repeat the overlap.
		if end == len(words) {
			break
		}
	}

	return chunks, nil
}
