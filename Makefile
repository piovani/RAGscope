.PHONY: help build test vet fmt tidy validate corpus up up-reranker models down vuln clean

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
	@echo "  up-reranker Start the TEI reranker too (x86 only)"
	@echo "  models      Download the Ollama embedding model (bge-m3)"
	@echo "  down        Stop all services"
	@echo "  vuln        Run govulncheck"
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

# Ollama runs on the host (it uses the Mac GPU), not in Docker.
models:
	ollama pull bge-m3

down:
	docker compose -f deployments/docker-compose.yml --profile reranker down

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

clean:
	rm -rf bin
