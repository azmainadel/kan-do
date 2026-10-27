# kan-do: a Go server with a React UI built into it.
GOTOOLCHAIN ?= go1.26.10
export GOTOOLCHAIN

BIN := kando
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/azmainadel/kan-do/internal/version.Version=$(VERSION:v%=%)

.PHONY: all ui build test lint e2e dev clean

all: ui build

ui: ui/node_modules
	cd ui && npm run build

ui/node_modules: ui/package-lock.json
	cd ui && npm ci
	@touch $@

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/kando

test: ui/node_modules
	go test -race ./...
	cd ui && npm test

lint: ui/node_modules
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
	test -z "$$(gofmt -l .)"
	cd ui && npm run lint && npm run typecheck

e2e: ui/node_modules
	cd ui && npm run e2e

# The UI with hot reload, against a board started with `kando --no-open`.
dev: ui/node_modules
	cd ui && npm run dev

clean:
	rm -rf $(BIN) internal/web/dist/app ui/test-results ui/playwright-report
