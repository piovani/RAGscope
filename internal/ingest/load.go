package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/piovani/RAGscope/internal/domain"
)

// LoadDir reads every .txt file in dir as a document. The file name (without
// the extension) becomes the document ID and title.
func LoadDir(dir string) ([]domain.Document, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no .txt files found in %s", dir)
	}

	docs := make([]domain.Document, 0, len(paths))
	for _, path := range paths {
		text, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		name := strings.TrimSuffix(filepath.Base(path), ".txt")
		docs = append(docs, domain.Document{
			ID:     name,
			Title:  name,
			Source: path,
			Text:   string(text),
		})
	}
	return docs, nil
}
