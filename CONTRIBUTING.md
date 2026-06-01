# Contributing to PHP LSP

Thank you for your interest in contributing! This document covers everything you need to get started.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## Getting Started

### Prerequisites

- **Go 1.22+** — for the language server

> The editor extensions live in separate repos ([`Tusk-PHP/vscode`](https://github.com/Tusk-PHP/vscode), [`Tusk-PHP/zed`](https://github.com/Tusk-PHP/zed)); their toolchains (Node.js, Rust) are documented there.

### Development Setup

```bash
git clone https://github.com/Tusk-PHP/lsp.git tusk-php
cd tusk-php

# Build the server
make build

# Run tests
make test

# Run locally with debug logging
make dev
```

### Project Structure

```
cmd/tusk-php/      Entry point
internal/          All server packages (parser, symbols, hover, completion, etc.)
testdata/          Test fixtures (mock PHP project)
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed documentation of internals.

## Development Workflow

### Running Tests

```bash
# All tests with race detection
make test

# Single test
go test -v -race -run TestHoverMethodChain ./internal/hover/

# Specific package
go test -v -race ./internal/symbols/
```

### Building Editor Extensions

The editor extensions are built and released from their own repositories:
[`Tusk-PHP/vscode`](https://github.com/Tusk-PHP/vscode) and
[`Tusk-PHP/zed`](https://github.com/Tusk-PHP/zed). See each repo's README for its
build steps; this repo only produces the `tusk-php` language server binary that
those extensions download.

### Testing with an Editor

1. Build the binary: `make build`
2. Point your editor to `build/tusk-php`
3. For VS Code: set `phpLsp.executablePath` to the absolute path of `build/tusk-php`
4. Use `make dev` to run with logging to `/tmp/tusk-php.log`

### Testing with Zed

The Zed extension can either find `tusk-php` on `PATH`, download the release
binary for the extension version, or use an explicit local binary path. For
local LSP development, prefer the explicit path so Zed runs the binary you just
built instead of downloading one from GitHub.

Build the local server:

```bash
make build
```

Then add this to your Zed settings, using the absolute path to this checkout:

```json
{
  "languages": {
    "PHP": {
      "language_servers": ["tusk-php"]
    }
  },
  "lsp": {
    "tusk-php": {
      "binary": {
        "path": "/absolute/path/to/tusk-php/build/tusk-php",
        "arguments": ["--transport", "stdio"]
      }
    }
  }
}
```

For this repository on a typical local checkout, that path may look like:

```json
{
  "lsp": {
    "tusk-php": {
      "binary": {
        "path": "/Users/d8vjork/Projects/OpenSoutheners/php-lsp/build/tusk-php",
        "arguments": ["--transport", "stdio"]
      }
    }
  }
}
```

After changing the binary path or rebuilding the server, run Zed's
`lsp: restart language servers` action. If the configured `binary.path` exists,
the Zed extension uses it and skips the online release download path.

## Making Changes

### Before You Start

- Check [existing issues](https://github.com/Tusk-PHP/lsp/issues) to avoid duplicate work
- For larger changes, open an issue first to discuss the approach

### Pull Request Process

1. Fork the repository and create a feature branch from `main`
2. Write tests for new functionality
3. Ensure `make test` passes with no failures
4. Keep commits focused — one logical change per commit
5. Open a pull request against `main`

### Code Style

- Go: standard `gofmt` formatting (enforced by CI)
- TypeScript: project `tsconfig.json` settings
- No external Go dependencies — the server has zero `require` directives in `go.mod` by design

### Test Fixtures

Tests use `testdata/project/` which contains a mock PHP project with `composer.json`, source files, and vendor stubs. When adding test cases:

- Add PHP fixtures to `testdata/project/src/` or `testdata/project/vendor/`
- Index fixtures in your test setup and assert against hover content, symbol resolution, etc.

## Reporting Bugs

- Use the [GitHub issue tracker](https://github.com/Tusk-PHP/lsp/issues)
- Include your editor, OS, PHP version, and steps to reproduce
- Attach the server log (`--log /tmp/tusk-php.log`) if relevant

## Feature Requests

Open an issue with the **feature request** label. Describe the use case and expected behavior.

## Release Process

Releases are automated via GitHub Actions. Pushing a semver tag (e.g., `v0.5.0`) triggers:

1. Full test suite
2. Cross-platform binary builds
3. GitHub Release with the binaries, `checksums.txt`, and changelog notes

The editor extensions release independently from their own repos ([`Tusk-PHP/vscode`](https://github.com/Tusk-PHP/vscode), [`Tusk-PHP/zed`](https://github.com/Tusk-PHP/zed)), each pinning a known-good LSP version.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
