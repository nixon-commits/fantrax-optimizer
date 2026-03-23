.PHONY: build test run dry-run

build:
	go build ./...

test:
	go test ./internal/...

run:
	go run ./cmd

dry-run:
	go run ./cmd --dry-run
