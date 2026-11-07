package httpjson

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithHeaderSendsHeaderWithoutChangingTheOriginal(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	plain := New(server.URL)
	withKey := plain.WithHeader("X-Api-Key", "secret")

	var out map[string]any
	if err := withKey.Post(context.Background(), "/x", map[string]string{}, &out); err != nil {
		t.Fatal(err)
	}
	if got.Get("X-Api-Key") != "secret" {
		t.Errorf("header not sent: %v", got)
	}
	if got.Get("Content-Type") != "application/json" {
		t.Errorf("content type lost: %v", got)
	}

	if err := plain.Post(context.Background(), "/x", map[string]string{}, &out); err != nil {
		t.Fatal(err)
	}
	if got.Get("X-Api-Key") != "" {
		t.Errorf("original client sent the header: %v", got)
	}
}
