.PHONY: build test test-integration api imports run up

build:
	mkdir -p bin
	go build -o bin/memex-server ./cmd/server
	go build -o bin/memexctl ./cmd/memexctl

test:
	go test ./...
	go run ./scripts/checkimports.go

test-integration:
	MEMEX_TEST_DSN='postgres://memex:memex@127.0.0.1:5433/memex_test?sslmode=disable' go test -count=1 -tags integration ./internal/api

api:
	go run ./cmd/openapi

imports:
	go run ./scripts/checkimports.go

up:
	docker compose -f deploy/compose.private.yml up -d --build

run:
	go run ./cmd/server
