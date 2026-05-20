# Current Issues

Issues discovered during orchestrated implementation and review work. Each open
entry records **Where**, **What**, and a suggested **Fix**, ordered by severity.

---

## Critical

_None open._

---

## High

_None open._

---

## Medium

### M5 — Parser errors are collected but not yet surfaced as diagnostics
- **Where:** `internal/parser/compat.go`, `internal/diagnostics/provider.go`
- **What:** `ParseResult.Errors` were already collected by the parser, and `ParseFile` now preserves them on `FileNode.Errors`, but the diagnostics pipeline still does not read either source. Syntax failures therefore remain internal parser state instead of editor-visible diagnostics.
- **Suggested Fix:** add a fast syntax-diagnostics pass in `internal/diagnostics` that reads `FileNode.Errors` (or `ParseResult.Errors` directly), converts each entry into a protocol diagnostic, and add protocol/provider tests that assert publication for unterminated strings/comments and structural recovery errors like missing `)` / `}`.

### M6 — Initial scope collector does not yet model arrow-function captures or grouped `use` imports
- **Where:** `internal/scope/collector.go`
- **What:** The new Sprint 1 collector now covers function/method/closure scopes, parameters,
  `foreach`, `catch`, destructuring, closure `use (...)`, and basic top-level imports. It does
  not yet infer implicit arrow-function captures (`fn () => $x`) and only records one alias per
  parsed `use` statement, so grouped imports such as `use Foo\{Bar, Baz};` are still outside the
  reusable collector surface.
- **Suggested Fix:** add an expression-range walker for `fn` bodies and either extend the parser's
  import model for grouped `use` statements or collect grouped aliases directly from the token
  stream before later source-context and diagnostics work starts depending on them.

### M7 — CursorContext scope only models enclosing class/method, not full position scope
- **Where:** `internal/source/context.go`
- **What:** Phase 1's shared `CursorContext` exposes `Scope`, but `scopeAt` only fills class and method metadata. Positions inside free functions, closures, and other non-method scopes therefore do not get a reusable scope descriptor, which falls short of ROADMAP Architecture Priority 1's "What scope contains the position?" target.
- **Suggested Fix:** extend `internal/source` scope derivation to recognize free functions and closures at minimum, or thread the later reusable scope collector into `CursorContext` once that API is stable.

### M8 — CursorContext namespace/import state is file-global rather than position-aware
- **Where:** `internal/source/context.go`
- **What:** `Analyze` currently reads `file.Namespace` and `file.Uses` directly from the parsed file and returns them for every position. That is adequate for the common single-namespace file, but it will misclassify positions in files with multiple namespace blocks or any future position-sensitive import handling because the shared context does not filter namespace/import state by cursor location.
- **Suggested Fix:** make namespace and active-import collection position-aware in `internal/source`, either by recording namespace/use ranges in the parser model or by deriving the active block from the token stream during analysis.

### M9 — `list(...)` destructuring is outside the collector binding surface
- **Where:** `internal/scope/collector.go`
- **What:** `parseDestructureBindings` only activates on `[` ... `] =` patterns. PHP's
  `list($a, $b) = ...` form is not recognized, so references/rename built on the collector miss
  one of the core destructuring syntaxes called out by the scope-collector roadmap slice.
- **Suggested Fix:** add a `list`-aware destructuring parser that treats `list(` headers as
  declaration sites, ideally sharing the same nested-binding extraction path used for bracket
  destructuring and `foreach` destructuring.

### M10 — `PrepareRename` still advertises any `$variable` even when no scoped binding exists
- **Where:** `internal/analyzer/analyzer.go`
- **What:** `PrepareRename` returns success for every non-`$this` variable token before checking
  whether `internal/scope` can resolve a binding at that position. `Rename` then correctly returns
  `nil` when the collector cannot resolve the symbol, so clients can be told rename is available
  for undefined locals, superglobals, or otherwise untracked variables and then fail on execution.
- **Suggested Fix:** route variable prepare-rename through `scope.Collect(source).BindingAt(pos)`
  and only allow rename when the collector resolves a local binding kind that the rename path can
  actually edit.

---

## Low / performance

_None open._

---

## Resolved

### C1 — Fatal stack overflow in the hover/completion chain resolver
- Fixed by an atomic re-entrancy depth guard (`const maxResolveDepth = 32`) added to
  `resolve.Resolver`; `ResolveVariableType` and `ResolveVariableTypeTyped` bail out once depth
  exceeds the bound. Regression tests in `internal/resolve/recursion_test.go`. The conformance
  harness now exercises hover/completion on member-access anchors (`->`, `?->`, `::`) with no overflow.

### H1–H4 — PHP 8.3–8.5 syntax silently dropped
- Fixed in `internal/parser/parser.go`: **H1** property hooks (`{ get => …; set => …; }`) are
  parsed brace-balanced and recorded as `PropertyDef.Hooks`; **H2** the `|>` pipe operator
  tokenizes to a dedicated `TokenPipeArrow`; **H3** asymmetric visibility (`public private(set) …`)
  is consumed as a modifier and recorded as `PropertyDef.SetVisibility`; **H4** dynamic class
  constant fetch (`Class::{$name}`) parses cleanly. Tests in `internal/parser/modern_syntax_test.go`.

### M1 — Completion returned items with an empty `Label`
- `GetCompletions` now routes every return path through a `sanitizeCompletions` helper that
  drops empty-`Label` items; `symbols.SearchByFQNPrefix` was also hardened to never return
  empty-name symbols. `internal/completion/provider.go`, `internal/symbols/index.go`.

### M2 — Non-deterministic completion ordering
- `sanitizeCompletions` sorts results deterministically (`SortText` → `Label` → `Kind` →
  `Detail`), preserving the existing priority buckets. Two identical calls now produce
  byte-identical output. `internal/completion/provider.go`.

### M3 — SignatureHelp `activeParameter` could exceed the parameter count
- `GetSignatureHelp` clamps `activeParameter` to `max(0, len(params)-1)`.
  `internal/analyzer/analyzer.go`.

### M4 — `lsp` `exit` notification called `os.Exit(0)` directly
- `Server` gained an injectable `exitFunc func(int)` field (defaults to `os.Exit`, set in
  `NewServer`); the `exit` handler calls it, making the full lifecycle testable.
  `internal/lsp/server.go`.

### L1 — `SearchByFQNPrefix` was O(N) over all symbols
- Replaced with binary search over a maintained sorted-FQN slice (dirty-flag rebuild done
  under the write lock — which also fixed the same latent rebuild race in the pre-existing
  `SearchByPrefix`). `internal/symbols/index.go`.

### L2 — `FindReferences` re-read every indexed file from disk per call
- `symbols.Index` now stores per-URI source in memory (`GetFileSource`), populated by
  `IndexFileWithSource` and `IndexIDEHelperFile`; `findSymbolOccurrences` uses it and falls
  back to disk only when no in-memory copy exists. `internal/symbols/index.go`,
  `internal/analyzer/analyzer.go`.

### L3 — `handleDidOpen` / `handleDidChange` indexed synchronously on the message loop
- Documents larger than `largeDocThreshold` (5000 lines) are indexed asynchronously via
  `goSafe`; smaller documents stay synchronous (so ordinary use and tests are deterministic).
  `internal/lsp/server.go`.

### L4 — strict-mode goroutine re-panic was undocumented
- `SetStrict` and `recoverPanic` now carry doc comments explaining that strict mode
  (`--strict` / `TUSK_STRICT`) causes fatal termination on any recovered panic, including
  from background goroutines. `internal/lsp/server.go`.

### L5 — `FuzzTokenize` was not run by the conformance workflow
- Added a nightly `FuzzTokenize` step (bounded `-fuzztime`) alongside `FuzzParseFile` and
  `FuzzIndexFile`. `.github/workflows/conformance.yml`.

### L6 — Corpus manifest refs
- Corrected in `testdata/corpus/manifest.json`: `guzzlehttp/{guzzle,psr7}` repo URLs → the
  `guzzle/*` GitHub org, `symfony/string` → `v7.2.6`, `tempestphp/tempest` → the `v1.5.0` tag.
  The conformance CI job now fetches the corpus successfully.

### L7 — Chain-resolver depth-guard amplification
- Fixed by adding a `break` after the `ChainResolver` call in `ResolveVariableType`, matching
  `ResolveVariableTypeTyped` — the scan stops at the nearest preceding assignment, removing the
  per-line re-invocation that amplified pathological cost.

### L8 — PHP 8.4 property metadata (hooks, set-visibility) threaded through to hover
- `PropertyNode` and `symbols.Symbol` gained `Hooks` and `SetVisibility` fields, populated
  through `toPropertyNode` and the symbol indexer; hover cards now render asymmetric visibility
  (`public private(set) …`) and `{ get; set; }` hooks. `internal/parser/compat.go`,
  `internal/symbols/index.go`, `internal/hover/format.go`.
