.PHONY: help build test vet fmt tidy validate up up-models down vuln clean

help:
	@echo "Targets:"
	@echo "  build       Compile bin/ragscope"
	@echo "  test        Run tests with the race detector"
	@echo "  vet         Run go vet"
	@echo "  fmt         Format code"
	@echo "  tidy        go mod tidy"
	@echo "  validate    Validate all experiment configs"
	@echo "  up          Start Postgres (pgvector)"
	@echo "  up-models   Start Postgres + TEI embedding/reranker"
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

up:
	docker compose -f deployments/docker-compose.yml up -d

up-models:
	docker compose -f deployments/docker-compose.yml --profile models up -d

down:
	docker compose -f deployments/docker-compose.yml --profile models down

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

clean:
	rm -rf bin
