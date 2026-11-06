// Package tei is a small HTTP client for Hugging Face Text Embeddings
// Inference. Embeddings and reranking use the same server software, so both
// share this code.
package tei

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

// BatchSize is TEI's default limit of inputs per request.
const BatchSize = 32

type Client struct {
	url  string
	http *http.Client
}

func New(url string) *Client {
	return &Client{
		url:  strings.TrimRight(url, "/"),
		http: &http.Client{Timeout: 2 * time.Minute},
	}
}

// Post sends request as JSON to path and decodes the JSON answer into response.
func (c *Client) Post(ctx context.Context, path string, request, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call TEI server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("TEI server returned %s: %s", resp.Status, msg)
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return fmt.Errorf("read TEI response: %w", err)
	}
	return nil
}
