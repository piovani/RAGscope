"""Local reranker server for machines where the TEI image does not run (Apple Silicon).

It speaks the same API as Hugging Face Text Embeddings Inference, so the Go
client in internal/rerank works with it unchanged:

    POST /rerank  {"query": "...", "texts": ["...", "..."]}
               -> [{"index": 0, "score": 0.93}, {"index": 1, "score": 0.12}]

Run it with `make rerank`. Settings (environment variables):
    RERANK_MODEL  cross-encoder to load (default BAAI/bge-reranker-v2-m3)
    PORT          port to listen on     (default 8082)
"""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import torch
from sentence_transformers import CrossEncoder

MODEL_NAME = os.environ.get("RERANK_MODEL", "BAAI/bge-reranker-v2-m3")
PORT = int(os.environ.get("PORT", "8082"))

# Use the Mac GPU when there is one.
DEVICE = "mps" if torch.backends.mps.is_available() else "cpu"

print(f"loading {MODEL_NAME} on {DEVICE} ...", flush=True)
model = CrossEncoder(MODEL_NAME, device=DEVICE, max_length=512)
print("ready", flush=True)

# The model is not safe to call from several threads at once.
lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    def _send(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/health":
            self._send(200, {"status": "ok", "model": MODEL_NAME, "device": DEVICE})
        else:
            self._send(404, {"error": "not found"})

    def do_POST(self):
        if self.path != "/rerank":
            self._send(404, {"error": "not found"})
            return

        try:
            length = int(self.headers.get("Content-Length", 0))
            request = json.loads(self.rfile.read(length))
            query, texts = request["query"], request["texts"]
        except (ValueError, KeyError):
            self._send(400, {"error": "expected JSON with 'query' and 'texts'"})
            return

        with lock:
            scores = model.predict([(query, text) for text in texts], batch_size=8)

        ranked = sorted(
            ({"index": i, "score": float(score)} for i, score in enumerate(scores)),
            key=lambda item: item["score"],
            reverse=True,
        )
        self._send(200, ranked)

    def log_message(self, *args):
        pass  # keep the terminal quiet


if __name__ == "__main__":
    print(f"listening on :{PORT}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
