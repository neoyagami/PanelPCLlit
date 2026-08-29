.PHONY: build test run

build:
	go build -trimpath -ldflags="-s -w" -o build/panelpc ./cmd/panelpc

test:
	go test ./...

run:
	go run ./cmd/panelpc
