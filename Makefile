.PHONY: generate build test docker-build docker-up docker-down functional-test lint clean help

help:
	@echo "Targets:"
	@echo "  generate         Generate protobuf code"
	@echo "  build            Build all packages"
	@echo "  test             Run unit tests"
	@echo "  lint             Lint proto and Go code"
	@echo "  docker-build     Build Docker images"
	@echo "  docker-up        Start Docker containers"
	@echo "  docker-down      Stop Docker containers"
	@echo "  functional-test  Run functional tests (starts/stops Docker)"
	@echo "  clean            Remove generated files and volumes"

generate:
	buf generate

build: generate
	go build ./...

test:
	go test ./internal/... -v

lint:
	buf lint
	go vet ./...

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

functional-test: docker-up
	go test ./tests/... -v -tags=functional -count=1
	docker compose down

clean:
	rm -rf gen/lbsw
	docker compose down -v 2>/dev/null || true
