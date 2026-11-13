.PHONY: build install test clean dev cross-build wasm conformance conformance-pr bench bench-install bench-self field-run field-baseline field-baseline-update

VERSION ?= 0.14.0
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

wasm:
	GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build $(LDFLAGS) -trimpath -o build/$(BINARY).wasm ./cmd/tusk-php/

conformance:
	bash scripts/fetch-corpus.sh --tier all
	go test -tags=conformance -race ./internal/conformance/...

conformance-pr:
	bash scripts/fetch-corpus.sh --tier pr
	go test -tags=conformance -race ./internal/conformance/...

bench-install:
	bash bench/tools/install.sh

bench: build
	go run ./bench/lspbench

bench-self: build
	go run ./bench/lspbench -servers tusk-php

# Diagnostics over a real project, as the server reports them:
#   make field-run ROOT=../framework PATHS='src/Illuminate' BASELINE=laravel.baseline
field-run: build
	./build/$(BINARY) check --root $(ROOT) $(if $(BASELINE),--baseline $(BASELINE)) $(PATHS)

# Diagnostics over Laravel, Symfony, WordPress and Magento 2 at pinned commits,
# compared with testdata/field/*.baseline. NAME=laravel checks one.
field-baseline: build
	bash scripts/field-baseline.sh $(NAME)

field-baseline-update: build
	bash scripts/field-baseline.sh --update $(NAME)
