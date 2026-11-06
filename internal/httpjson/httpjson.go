// Package httpjson is a tiny client for servers that talk JSON over HTTP POST
// (Ollama, the reranker). It exists so each adapter does not repeat the same
// request, status check and decoding code.
package httpjson

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

type Client struct {
	url  string
	http *http.Client
}

func New(url string) *Client {
	return &Client{
		url:  strings.TrimRight(url, "/"),
		http: &http.Client{Timeout: 5 * time.Minute}, // local models can be slow on the first call
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
		return fmt.Errorf("call %s: %w", c.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s returned %s: %s", c.url, resp.Status, msg)
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return fmt.Errorf("read answer from %s: %w", c.url, err)
	}
	return nil
}
