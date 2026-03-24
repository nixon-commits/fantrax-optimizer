.PHONY: build install test run dry-run

build:
	go build -o rosterbot .

install:
	go install .

test:
	go test ./internal/...

run:
	go run . optimize

dry-run:
	go run . optimize --dry-run
