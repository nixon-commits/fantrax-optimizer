.PHONY: build install test run dry-run

build:
	go build -o rosterbot .

install:
	go install .
	"$$(go env GOPATH)/bin/rosterbot" completion zsh > "$${HOMEBREW_PREFIX:-/usr/local}/share/zsh/site-functions/_rosterbot"

test:
	go test ./internal/...

run:
	go run . optimize

dry-run:
	go run . optimize --dry-run
