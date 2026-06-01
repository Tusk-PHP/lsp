.PHONY: build install test clean dev cross-build conformance conformance-pr

VERSION ?= 0.5.0
BINARY  := tusk-php
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -trimpath -o build/$(BINARY) ./cmd/tusk-php/

install: build
	cp build/$(BINARY) $(HOME)/.local/bin/$(BINARY)

dev:
	go run ./cmd/tusk-php/ --log /tmp/tusk-php.log

test:
	go test -v -race ./...

clean:
	rm -rf build/ dist/

cross-build:
	bash scripts/build.sh

conformance:
	bash scripts/fetch-corpus.sh --tier all
	go test -tags=conformance -race ./internal/conformance/...

conformance-pr:
	bash scripts/fetch-corpus.sh --tier pr
	go test -tags=conformance -race ./internal/conformance/...
