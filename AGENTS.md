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
awareness; WordPress and Magento 2 support are in progress (see the pinned
[Roadmap](https://github.com/Tusk-PHP/lsp/issues/39)). One binary, `tusk-php`,
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
make field-baseline   # Laravel, Symfony, WordPress, Magento 2 against testdata/field/*.baseline (NAME=laravel for one)
make field-baseline-update   # rewrite those baselines after an intended change

go test -v -race -run TestHoverMethodChain ./internal/hover/   # one test
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

1. **Wire types:** `protocol` (LSP types, no logic).
2. **Language core:** `parser` (own tokenizer and lightweight AST, fault-tolerant),
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

Known exception to fix: `config` imports `checks` for its `Severity` type. Don't
add more; the type belongs in a lower layer.

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
├── document/         open documents: versions, immutable snapshots, edits, UTF-16 positions
├── indexing/         when and what to (re)index: scheduling, invalidation, caches
├── workspace/        roots, Composer discovery, project profile
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
  `lsp/server.go` (`handleInitialize`) and `mcpserver/server.go` (`buildRuntime`).
- **`document`** owns the latest text of each open file and its version. Analysis
  runs on an immutable snapshot and its result is dropped if a newer version
  arrived meanwhile.
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
  package (for example `lsp/server.go` into transport, dispatch, initialize,
  documents and handler files) before adding more. Don't create a new package
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
  Symfony, WordPress and Magento 2 at pinned commits to the baselines in
  `testdata/field/`: when a change adds or removes findings there, look at
  each one, then run `make field-baseline-update` and commit the result.
- The parser must stay fault-tolerant: bail-out conditions, progress guards,
  partial results on panic. Message handlers and goroutines recover panics
  (`recoverPanic`, `goSafe`); `-strict` / `TUSK_STRICT` re-panics in development.

## Before opening a pull request

1. `make test` passes (with `-race`).
2. `make conformance-pr` passes for changes to the parser, index or providers,
   and `make field-baseline` for changes to diagnostics or typing.
3. `make wasm` still builds.
4. New behaviour has a test that fails without it. Tests use the mock project in
   `testdata/project/`; corpus tests fetch real projects with
   `scripts/fetch-corpus.sh`.
5. Performance numbers are in the PR when the rules above ask for them.
6. `CHANGELOG.md` has an entry under `[Unreleased]` for anything a user would
   notice, written for users ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/)):
   what changed for them, not which functions moved.
7. Commit messages follow the existing style: `fix: …`, `feat: …`, `docs: …`,
   `chore: …`, `refactor: …`.

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
  no whole-parser rewrite while the current one can be extended.
