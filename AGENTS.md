# AGENTS.md

Guidance for anyone, human or AI agent, changing this repository. These are the
project's principles: follow them on every change, and when one gets in your way,
say so in the pull request instead of working around it quietly.

`CLAUDE.md` only points here. If your tool reads another file name, point it at
this file too rather than copying it. Some packages (`internal/completion`,
`internal/hover`, `internal/resolve`) have their own `AGENTS.md` with notes for
that package; read it before changing the package.

## What this is

Tusk PHP is a PHP language server written in Go, with deep Laravel and Symfony
awareness; WordPress and Magento 2 support are in progress (see the
[1.0.0 milestone](https://github.com/Tusk-PHP/lsp/milestone/9)). One binary, `tusk-php`,
serves:

- the Language Server Protocol over stdio (the default),
- an MCP server for AI agents (`tusk-php mcp`),
- CLI tools (`introspect`, `deps`, `-parse`),
- the same code built for WebAssembly (`wasip1`).

PHP 7.4 is the baseline for builtin symbols; newer versions and extensions are
layered on from the project's `composer.json`. Editor extensions live in separate
repos (`Tusk-PHP/vscode`, `Tusk-PHP/zed`) and pin a released server by SHA-256.

## Commands

```bash
make build            # build/tusk-php
make test             # go test -v -race ./...
make dev              # run locally, logging to /tmp/tusk-php.log
make wasm             # build/tusk-php.wasm (GOOS=wasip1)
make cross-build      # all release targets + checksums.txt
make conformance-pr   # corpus invariants used in CI (fetches the corpus first)
make conformance      # full corpus
make bench-install    # once: install the other servers for comparison
make bench-self       # benchmark this server only
make bench            # benchmark against the other servers
make field-run ROOT=../framework PATHS=src   # diagnostics over a real project, as the server reports them
make field-baseline   # frameworks and reference apps against testdata/field/*.baseline (NAME=laravel for one)
make field-baseline-update   # rewrite those baselines after an intended change
make reference-apps   # fetch the reference apps: Filament demo, Sylius, WooCommerce, Mage-OS
make bench-reference  # benchmark this server on them against bench/passmarks.json

go test -v -race -run TestHoverMethodChain ./internal/hover/   # one test
TUSK_PHP_TOKENIZERS='php;docker run --rm -i -v /tmp:/tmp:ro php:7.4-cli php' go test ./internal/php/lexer/   # lexer against each PHP's token_get_all
php scripts/generate-builtins.php registry --output=internal/symbols/builtin_registry_generated.go
php scripts/generate-builtins.php stubs --output=/tmp/php-builtins.generated.php
```

Project config is `.tusk-php.json` in the workspace root, merged under the
client's `initializationOptions`. Every option is documented in
`docs/configuration.md`.

## Architecture today

Entry point: `cmd/tusk-php/main.go`. The LSP server (`internal/lsp`) reads
JSON-RPC from stdin, `handleMessage()` dispatches by method to a provider, and
the provider answers from the shared symbol index. `ARCHITECTURE.md` has the
detail; this is the map.

Packages are layered. **A package may import only from its own layer or the ones
above it in this list**, never from a later one:

1. **Wire types and identifiers:** `protocol` (LSP types, no logic), `rule`
   (diagnostic codes, severities and sources: every code a finding carries is a
   `rule.Code` constant listed in `rule.All`), `strict` (whether a recovered
   panic is raised again: `-strict` or `TUSK_STRICT`).
2. **Language core:** `php/token`, `php/lexer`, `php/ast` and `php/parser` (a
   lossless lexer and a full fault-tolerant parser that match the tokenizer and
   parser of PHP 7.4 and 8.0 to 8.6, with `short_open_tag` off, so `<?` is
   inline HTML; see issue #140; the lexer leaves out the empty
   `T_ENCAPSED_AND_WHITESPACE` PHP emits for a space in an unquoted string offset
   like `"$a[ b]"`), `parser` (the older tokenizer and lightweight AST the
   features still use),
   `types`, `phparray`, `stubs` (embedded builtin stubs by PHP version and
   extension), `phpdetect`.
3. **Index and resolution:** `symbols` (the central index), `scope` (variables,
   closures, imports), `resolve` (types, PHPDoc, generics), `source` (what is under
   the cursor).
4. **Project context:** `config`, `composer`, `coverage`, `framework/laravel`,
   `framework/symfony`, `framework/magento`, `container`, `models`, `workspace`
   (builds the index for a root: files, Composer dependencies, framework analysis).
5. **Features:** `completion`, `hover`, `analyzer` (definition, references,
   symbols, rename, signature help), `inlayhint`, `checks` (native rules, testable
   without LSP types), `diagnostics` (runs `checks`, plus PHPStan and Pint),
   `symquery`, `introspect`, `deps`, `unuseddeps`, `graph`, `composer/cardhover`.
6. **Transports:** `lsp`, `mcpserver`.
7. **Tests across everything:** `conformance`.

Builtins are registered in a fixed order, later layers overriding earlier ones:
generated registry → PHP 7.4 core baseline → core version deltas up to the
selected version → extension baselines → extension deltas → hand-written
compatibility entries.

## Target structure

This is where the code is going. Each new package arrives with the first change
that gives it a real job; don't create empty packages ahead of time, and don't
move code just to match this tree.

```
internal/
├── protocol/         LSP wire types only
├── lsp/              LSP transport: read, schedule, write, translate to/from protocol
├── mcpserver/        MCP transport: tools and resources only
├── session/          builds and connects the analysis providers once, for every transport
├── document/         open documents: versions, immutable snapshots, edits, column conversion
├── indexing/         when and what to (re)index: scheduling, invalidation, caches
├── workspace/        roots, Composer discovery, project profile
├── php/              token, lexer, ast, parser: PHP's own grammar, version by version (issue #140)
├── parser/ types/ symbols/ scope/ resolve/ source/      language core (no transport types over time)
├── completion/ hover/ analyzer/ inlayhint/ checks/ diagnostics/   features
├── framework/
│   ├── laravel/  symfony/  wordpress/  magento/
├── container/ models/ composer/ config/ stubs/ …
```

What each piece owns once it exists:

- **`session`** is the only place that constructs providers and wires resolvers
  between them. `lsp` and `mcpserver` ask it for providers and add only
  transport-specific parts (client commands, notifications, tool registration).
  Until it exists, any wiring change must be made in both
  `lsp/initialize.go` (`handleInitialize`) and `mcpserver/server.go` (`buildRuntime`).
- **`document`** owns the latest text of each open file and its version. Analysis
  runs on an immutable snapshot and its result is dropped if a newer version
  arrived meanwhile. It also converts columns between the byte offsets the core
  uses and the position encoding negotiated with the client (UTF-8 or UTF-16).
- **`indexing`** owns scheduling and invalidation; `symbols` keeps the data and the
  queries.
- **`lsp`** owns the single output writer and request scheduling (bounded workers,
  `$/cancelRequest`, interactive requests before background work).
- **Framework knowledge** lives under `framework/<name>/`, never inside generic
  completion, hover or checks.
- **Core packages** move to protocol-neutral positions and ranges over time,
  converted at the transport edge. New core code should not add new uses of
  `protocol` types.

## Structure rules

- Respect the layer order above. If you need something from a later layer, move
  the shared piece down; don't import up.
- Reuse the shared foundations: cursor context from `source`, variables from
  `scope`, types and PHPDoc from `resolve`. Don't re-derive them with string
  scanning in a feature.
- Keep files to one responsibility. Past roughly 800 lines, split within the same
  package (as `lsp` is split into transport, dispatch, initialize, documents,
  commands and handler files) before adding more. Don't create a new package
  just to split a file.
- Platform differences go in build-tagged files (`*_wasip1.go`), not runtime
  checks. Everything must still build with `make wasm`.
- No global mutable state and no goroutine-identity tricks. Per-request state is
  passed explicitly (a `context.Context` or a request struct).
- Advertise an LSP capability only once it works end to end.
- No new dependency unless it's mature and maintained; say why in the PR.

## Performance rules

Tusk PHP's lead is speed and memory: about 0.3 s and 160 MB to the first
cross-file answer on a 9,700-file Laravel app. Every change must keep it.

**Never block typing**

- The message loop does no work that grows with the workspace. Anything
  proportional to project size (indexing, workspace-wide diagnostics, reference
  scans) runs in the background through `goSafe`, and its result is checked
  against the document version before it is used.
- Work done on `didChange` grows with the edited file only. Expensive diagnostics
  are debounced or run on save; external tools (PHPStan, Pint) only run on save.
- Latency-sensitive requests (completion, hover, signature help) are never
  debounced and never wait behind background work.

**Lookups, not scans**

- Request paths look things up in maps. A loop over every symbol, file or
  inheritance edge in a request path is a bug: add an index (for example a
  reverse map from parent to children) instead.
- Don't read files from disk or re-tokenize the same source more than once per
  request. Tokenize once and share the tokens between rules.
- Compile regular expressions once, at package level. Prefer tokens over regular
  expressions for anything that must understand PHP.

**Locks**

- Hold the index lock only to read or swap data. Build a file's new symbols
  outside the lock, then replace that file's entries in one short critical
  section.
- Never do I/O, call a provider or call back into user code while holding a lock.
- Don't hand out internal slices or maps for callers to modify; return copies or
  document them as read-only.
- Output to the client goes through one writer; never write to the stream from
  several goroutines without it.

**Memory**

- Every cache is bounded and has an owner that invalidates it.
- Don't keep data you can rebuild cheaply, especially raw source for vendor files.
- The WebAssembly build has to stay under about 448 MB: think about what a change
  keeps per file times 50,000 files.

**Allocations on hot paths**

- Tokenizing, resolving and completion are hot. Preallocate slices when the size
  is known, use `strings.Builder`, avoid `fmt` and `strings.Split` of whole files
  in loops, and avoid converting between `[]byte` and `string` repeatedly.

**Measure, don't guess**

- A change to indexing, resolution, the parser or the request path needs a number
  in the PR: `make bench-self` before and after, or a `go test -bench` benchmark
  with `-benchmem` for the function you changed.
- A change that makes the benchmark more than about 10% slower or heavier, or that
  lowers its count of correct answers, needs a reason in the PR.
- Optimize from a profile (`go test -cpuprofile` / `-memprofile`, `go tool pprof`),
  not from intuition.

## Correctness rules

- A false warning costs more than a missing one. When a rule can't tell, it stays
  quiet.
- Rules that need to understand PHP read tokens or the AST, not raw lines. Code
  outside `<?php … ?>`, strings, heredocs and comments is never checked as code.
- Respect the project's PHP version and extensions (from `composer.json`) in
  every rule that depends on them.
- Real codebases beat fixtures: before shipping a diagnostic change, run it over a
  real project (a Laravel or Symfony app, WordPress, Magento 2) and look at what it
  reports, not only at the tests. `tusk-php check` (or `make field-run`) reports
  what the server would: run it before and after, or keep a `--write-baseline`
  from `main` and check against it with `--baseline`. CI holds Laravel,
  Symfony, WordPress and Magento 2, and a reference app for each (the Filament
  demo, Sylius, WooCommerce, Mage-OS), at pinned versions to the baselines in
  `testdata/field/`: when a change adds or removes findings there, look at
  each one, then run `make field-baseline-update` and commit the result.
- The parser must stay fault-tolerant: bail-out conditions, progress guards,
  partial results on panic. Message handlers and goroutines recover panics
  (`recoverPanic`, `goSafe`, and the parser's own guard); `-strict` / `TUSK_STRICT`
  re-panics in development, read through `strict.Enabled()`.

## Before opening a pull request

[CONTRIBUTING.md](CONTRIBUTING.md) is the source of truth for how changes get in:
the pull request checklist (tests, conformance, field baselines, `make wasm`,
performance numbers, the release note, commit style), keeping pull requests small,
drafts and stacks, review and releases. Read it and follow its checklist before
opening a pull request. This file covers the code's principles.

Pull requests don't edit `CHANGELOG.md`: put a **Release note** in the pull request
description instead, and the maintainers write the changelog from it when they release.

## Opening an issue

Follow [CONTRIBUTING.md › Issues, goals and milestones](CONTRIBUTING.md#issues-goals-and-milestones).
An issue whose result is a diagnostic goes to `Tusk-PHP/diagnostics`, not here
([Where an issue goes](CONTRIBUTING.md#where-an-issue-goes)); replace
`Tusk-PHP/lsp` with `Tusk-PHP/diagnostics` in the commands below for it.
An issue you open is a sub-issue of one goal (the open issues with the
[`roadmap`](https://github.com/Tusk-PHP/lsp/issues?q=is%3Aissue%20state%3Aopen%20label%3Aroadmap) label), has a type (Bug, Feature or Task) and an `area:` label where one fits. Link it
to its goal after creating it:

```bash
gh api -X POST repos/Tusk-PHP/lsp/issues/<goal>/sub_issues \
  -F sub_issue_id="$(gh api repos/Tusk-PHP/lsp/issues/<new> --jq .id)"
```

Never set a milestone: milestones are versions, and the maintainers decide what goes in
a release.

## Direction

- **Correct answers first**, then new features. The benchmark tracks both.
- **Shared foundations first**: `source`, `scope`, `resolve` exist so completion,
  hover, definition, references, rename, diagnostics and code actions agree.
- **Framework depth** is the differentiator: Laravel and Symfony first, then
  WordPress and Magento 2. Framework intelligence lives under `framework/`.
- **Native diagnostics** have stable codes and configurable severity; external
  tools stay separate from them.
- **Structured types**: PHPDoc handling moves to shared structured types (unions,
  intersections, generics, shapes, `@phpstan-type`, conditional returns).
- **Code actions** start with quick fixes (imports) before larger refactors.
- **Editors**: VS Code, Zed and Neovim are first-class. Zed users can't configure
  the server, so every default must be sensible.
- **Non-goals**: no native formatter (Laravel Pint stays the formatting path), and
  no big-bang parser rewrite. The current parser can't grow a statement and
  expression tree, so issue #140 replaces it one step at a time (lexer, parser,
  then moving callers over); the old parser stays until its last caller has moved.
