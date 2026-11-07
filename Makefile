.PHONY: help build test vet fmt tidy validate corpus up up-reranker rerank models down vuln lint clean

help:
	@echo "Targets:"
	@echo "  build       Compile bin/ragscope"
	@echo "  test        Run tests with the race detector"
	@echo "  vet         Run go vet"
	@echo "  fmt         Format code"
	@echo "  tidy        go mod tidy"
	@echo "  validate    Validate all experiment configs"
	@echo "  corpus      Regenerate data/raw/lgpd.txt from data/sources"
	@echo "  up          Start Postgres (pgvector)"
	@echo "  up-reranker Start the TEI reranker in Docker (x86 only)"
	@echo "  rerank      Run the local reranker server (Apple Silicon; needs .venv)"
	@echo "  models      Download the Ollama models (bge-m3 embeddings, llama3.2:3b answers)"
	@echo "  down        Stop all services"
	@echo "  vuln        Run govulncheck"
	@echo "  lint        Run golangci-lint"
	@echo "  clean       Remove build output"

build:
	go build -o bin/ragscope ./cmd/ragscope

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

validate: build
	./bin/ragscope validate configs/experiments/*.yaml

corpus: build
	./bin/ragscope convert data/sources/L13709.html data/raw/lgpd.txt

up:
	docker compose -f deployments/docker-compose.yml up -d

up-reranker:
	docker compose -f deployments/docker-compose.yml --profile reranker up -d

# The TEI image only exists for x86. On Apple Silicon run the same API locally.
# One-time setup: python3 -m venv .venv && .venv/bin/pip install -r scripts/requirements.txt
rerank:
	.venv/bin/python scripts/rerank_server.py

# Ollama runs on the host (it uses the Mac GPU), not in Docker.
models:
	ollama pull bge-m3
	ollama pull llama3.2:3b

down:
	docker compose -f deployments/docker-compose.yml --profile reranker down

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin
