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

	headers map[string]string // sent on every request, e.g. an API key
}

func New(url string) *Client {
	return &Client{
		url:  strings.TrimRight(url, "/"),
		http: &http.Client{Timeout: 5 * time.Minute}, // local models can be slow on the first call
	}
}

// WithHeader returns a copy of the client that also sends the header on every
// request. It is a copy so the original client keeps working unchanged.
func (c *Client) WithHeader(key, value string) *Client {
	headers := make(map[string]string, len(c.headers)+1)
	for k, v := range c.headers {
		headers[k] = v
	}
	headers[key] = value

	clone := *c
	clone.headers = headers
	return &clone
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
	for key, value := range c.headers {
		req.Header.Set(key, value)
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
