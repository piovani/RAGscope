package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TEI's default limit is 32 inputs per request, so we send texts in groups.
const batchSize = 32

// TEI talks to a Hugging Face Text Embeddings Inference server.
type TEI struct {
	url    string
	client *http.Client
}

func NewTEI(url string) *TEI {
	return &TEI{
		url:    strings.TrimRight(url, "/"),
		client: &http.Client{Timeout: 2 * time.Minute},
	}
}

func (t *TEI) Name() string { return "tei" }

func (t *TEI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))

	for start := 0; start < len(texts); start += batchSize {
		end := min(start+batchSize, len(texts))

		batch, err := t.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batch...)
	}

	return vectors, nil
}

func (t *TEI) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// truncate: cut texts that are too long instead of failing the whole request.
	body, err := json.Marshal(map[string]any{"inputs": texts, "truncate": true})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call embedding server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("embedding server returned %s: %s", resp.Status, msg)
	}

	var vectors [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&vectors); err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}
	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("sent %d texts but got %d vectors", len(texts), len(vectors))
	}
	return vectors, nil
}
