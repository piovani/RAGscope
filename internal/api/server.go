// Package api exposes the HTTP interface (health check now; query endpoint later).
package api

import (
	"encoding/json"
	"net/http"
)

// NewHandler returns the HTTP handler for the RAGscope API.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
