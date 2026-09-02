.PHONY: generate build test run lint

generate:
	buf generate

build: generate
	go build -o bin/sing-box-tui ./cmd/sing-box-tui

test:
	go test ./...

run: build
	./bin/sing-box-tui

lint:
	golangci-lint run ./...
