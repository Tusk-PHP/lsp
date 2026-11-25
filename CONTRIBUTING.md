# Contributing to Tusk PHP

Thank you for your interest in contributing! This document is the source of truth for how changes get into Tusk PHP: setup, the pull request checklist, review and releases. [AGENTS.md](AGENTS.md) holds the code's principles (structure, performance, correctness) and points AI agents back here for the process.

Questions and ideas go to [Discussions](https://github.com/Tusk-PHP/lsp/discussions); bugs and agreed work go to [issues](https://github.com/Tusk-PHP/lsp/issues).

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## Getting Started

### Prerequisites

- **Go 1.27+** — for the language server

> The editor extensions live in separate repos ([`Tusk-PHP/vscode`](https://github.com/Tusk-PHP/vscode), [`Tusk-PHP/zed`](https://github.com/Tusk-PHP/zed)); their toolchains (Node.js, Rust) are documented there.

### Development Setup

```bash
git clone https://github.com/Tusk-PHP/lsp.git tusk-php
cd tusk-php

# Build the server
make build

# Install the Laravel and Symfony fixtures' dependencies (needs Composer).
# Without them, the tests that read those frameworks' sources are skipped.
composer install --working-dir testdata/laravel --no-scripts
composer install --working-dir testdata/symfony --no-scripts

# Run tests
make test

# Run locally with debug logging
make dev
```

### Project Structure

```
cmd/tusk-php/      Entry point
internal/          All server packages, in layers (protocol, parser, symbols, resolve, features, transports)
testdata/          Test fixtures (mock PHP project, field baselines)
```

The packages are layered: a package may import only from its own layer or the ones above it. [AGENTS.md](AGENTS.md) lists the layers and the project's structure, performance and correctness rules, and every change, with or without AI tools, is reviewed against them. [ARCHITECTURE.md](ARCHITECTURE.md) documents the internals in detail.

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
3. For VS Code: set `tuskPhpLsp.executablePath` to the absolute path of `build/tusk-php`
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

After changing the binary path or rebuilding the server, run Zed's
`lsp: restart language servers` action. If the configured `binary.path` exists,
the Zed extension uses it and skips the online release download path.

## Benchmarking Against Other Servers

`bench/` holds a harness that asks tusk-php and four other PHP language servers
— Intelephense, Phpactor, php-lsp and PHPantom — the same editor questions in
the same fixture workspaces, then reports start-up time, memory, latency and how
many answers were correct.

```bash
make bench-install   # fetch the other servers into bench/.tools/ (gitignored)
make bench           # run everything; writes bench/results/<timestamp>/report.md
make bench-self      # tusk-php only, e.g. to check a change did not regress latency
```

Adding a case is a JSON entry in `bench/suite/*.json`; adding a server is a JSON
entry in `bench/servers.json`. See `bench/README.md` for the methodology and the
fairness rules the harness enforces.

## Making Changes

### Before You Start

- Look through [open issues](https://github.com/Tusk-PHP/lsp/issues) and [pull requests](https://github.com/Tusk-PHP/lsp/pulls) so you don't duplicate someone's work.
- **Recommended: work from an issue and get it assigned to you.** Comment on the issue you want to take (or open one), and once it's assigned to you, others know it's taken. If an issue is already assigned, ask the assignee before starting on it. It isn't mandatory, but it saves two people writing the same change. The maintainers work the same way and assign themselves before they start.
- **Talk first about anything that changes the architecture:** a new package, a new dependency, a change to [ARCHITECTURE.md](ARCHITECTURE.md) or core packages, or something AGENTS.md lists as a non-goal. The maintainers keep maintaining a feature long after its pull request is merged, so the core maintainer decides whether a feature is worth maintaining and how it fits the architecture. An issue or discussion first avoids work that can't be merged.

### Keep Pull Requests Small

- **One task per pull request.** Small, focused pull requests conflict less with each other, are faster to review and can be scheduled into releases separately. Open as many as you like.
- **Drafts are work in progress.** They aren't reviewed until you mark them ready.
- **Stacked pull requests:** when one builds on another, say so at the top of its description (`Stacked on #141`). A stack is treated as one task: it is reviewed in order and ships in one release.

### AI-Assisted Contributions

Contributions written with AI tools are welcome, and so are those written by hand. The maintainers use AI tools too, both to write changes and to help review them, and the same rules apply to everyone:

- You're responsible for what you submit: you've run it, read it, and can answer questions about it in review.
- AI agents follow [AGENTS.md](AGENTS.md) (`CLAUDE.md` points there), which sends them to this file for the process.
- Maintainers may use AI to help review a pull request, then test it by hand before approving it.

### Pull Request Checklist

1. Fork the repository and create a branch from `main`. Keep **Allow edits by maintainers** ticked so a maintainer can rebase it if another merge makes it conflict.
2. `make test` passes (with `-race`).
3. `make conformance-pr` passes for changes to the parser, index or providers, and `make field-baseline` for changes to diagnostics or typing. When a change adds or removes findings in the field baselines, look at each one, then run `make field-baseline-update` and commit the result.
4. `make wasm` still builds.
5. New behaviour has a test that fails without it.
6. Performance numbers are in the pull request when AGENTS.md's performance rules ask for them (`make bench-self` before and after, or a `go test -bench` with `-benchmem`).
7. `CHANGELOG.md` has an entry under `[Unreleased]` for anything a user would notice, written for users ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/)): what changed for them, not which functions moved.
8. Commits are focused, one logical change each, with messages in the existing style: `fix: …`, `feat: …`, `docs: …`, `chore: …`, `refactor: …`, `test: …`.
9. Open the pull request against `main`, linking the issue it closes.

### Review

- Pull requests are merged with **Rebase and merge**, so each commit lands on `main` as written.
- Changes to the paths in [`.github/CODEOWNERS`](.github/CODEOWNERS) need the core maintainer's review: the project principles and ARCHITECTURE.md, CI and release workflows, dependencies, build scripts and the field baselines.
- Architecture decisions are the core maintainer's. Expect questions on a pull request that adds packages or changes core code without an issue that agreed the approach.

### Code Style

- Go: standard `gofmt` formatting (enforced by CI)
- Go dependencies stay few: add one only when it is mature and maintained, and say why in the pull request

### Test Fixtures

Tests use `testdata/project/` which contains a mock PHP project with `composer.json`, source files, and vendor stubs. When adding test cases:

- Add PHP fixtures to `testdata/project/src/` or `testdata/project/vendor/`
- Index fixtures in your test setup and assert against hover content, symbol resolution, etc.

## Reporting Bugs

- Use the [GitHub issue tracker](https://github.com/Tusk-PHP/lsp/issues)
- Include your editor, OS, PHP version, and steps to reproduce
- Attach the server log (`--log /tmp/tusk-php.log`) if relevant
- Security vulnerabilities don't go in public issues: see [SECURITY.md](SECURITY.md)

## Feature Requests

Start a [discussion](https://github.com/Tusk-PHP/lsp/discussions) for an idea you want to talk through, or open an issue with the `enhancement` label for a concrete request. Describe the use case and the behaviour you expect.

## Release Process

Releases are automated via GitHub Actions. Pushing a semver tag without a `v` prefix (e.g., `0.16.0`) triggers:

1. Full test suite
2. Cross-platform binary builds
3. GitHub Release with the binaries, `checksums.txt`, and changelog notes

The editor extensions release independently from their own repos ([`Tusk-PHP/vscode`](https://github.com/Tusk-PHP/vscode), [`Tusk-PHP/zed`](https://github.com/Tusk-PHP/zed)), each pinning a known-good LSP version.

## Internals: releases

This section is for internals, the team behind Tusk PHP who review, merge and release. Contributors don't need to do any of this.

**Who the internals are.** Anyone can become one. The core maintainer (@d8vjork) decides, weighing the work a contributor has done on the project, community demand (issues, discussions) and the project's own workload (internal tasks, milestones and their scope).

**One tracking issue per release.** The next release always has an open issue titled `Release x.y.z` (for example [Release 0.16.0](https://github.com/Tusk-PHP/lsp/issues/152)). It lists:

- **Merged:** every PR merged into `main` since the last release, ticked.
- **Planned:** PRs in review that should ship with it, unticked. One that isn't ready by release time moves to the next release's issue. A stack of PRs is one item and ships together.
- **Not in this release:** open PRs deliberately left out, with the reason.
- **Before tagging:** the release checklist.

Choose the version from the changes, following [SemVer](https://semver.org/) while we're on `0.x`: a minor bump (`0.16.0`) when anything is added or changes behaviour, a patch bump (`0.15.1`) for fixes only.

**When you merge a PR:**

1. Merge with **Rebase and merge**. No squash merges or merge commits, so `main` keeps each commit as written.
2. If an earlier merge left the PR conflicting, rebase it onto `main` before merging (maintainers can push to contributors' PR branches), and run the tests for the packages involved.
3. Add the PR to the release issue: tick it under **Merged**, or add `- [x] #123 title` if it wasn't planned. Linking it there shows the release issue on the PR's timeline, so anyone looking at the PR can see which release it ships in.
4. Make sure the PR's `CHANGELOG.md` entry under `[Unreleased]` reads as a release note for users.

**When you release:**

1. Check that `[Unreleased]` in `CHANGELOG.md` covers every user-visible change in the issue's **Merged** list, then rename it to the version and date.
2. Check that CI, conformance and the field baselines are green on `main`.
3. Push the tag (see [Release Process](#release-process)).
4. Close the release issue with a link to the GitHub Release, and open the next `Release x.y.z` issue, moving any unticked **Planned** PRs into it.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
