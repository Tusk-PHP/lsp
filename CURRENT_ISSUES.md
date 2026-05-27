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

### M12 — `builtin-unavailable` constants scope not yet implemented
- **Where:** future `internal/checks/builtin_unavailable.go` (initial PR ships functions + class-likes only).
- **What:** `BUILTIN_STUBS_PLAN.md` calls for diagnostics on built-in constants (`JSON_THROW_ON_ERROR`, `SORT_FLAG_CASE`) and class constants. Deferred so the initial rule lands with the highest-impact scopes first.
- **Fix:** Extend `BuiltinUnavailableRule` with a constants scope and seed availability entries for those constants. Bitwise-or chains need per-token checking.

### M13 — `resolveSymbolAtCursor` still falls back to `LookupByName` in `->`/`::` context
- **Where:** `internal/analyzer/analyzer.go:665-666` (the broad `a.index.LookupByName(...)` + `PickBestStandalone` tail of `resolveSymbolAtCursor`).
- **What:** The hover and `FindDefinition` / `FindTypeDefinition` paths were hardened to return nil when the cursor is in `->`/`::` access context but the receiver type cannot be resolved (see W1 of the stubs-generics work). `resolveSymbolAtCursor` — used by `FindReferences`, `PrepareRename`, and `Rename` — still has the same fallthrough: when the access chain can't be resolved, it ignores `ctx.AccessKind` and matches any project-wide symbol with the same short name. Rename is the worst case: invoking rename on `$x->name` where `$x` has an unknown type can match and edit unrelated `name` symbols across the project.
- **Fix:** Mirror the W1 guard in `resolveSymbolAtCursor`: when `ctx.AccessKind != AccessNone` and the resolved chain FQN is empty, return `nil` rather than falling through to `LookupByName`. Add regression tests under `internal/analyzer/` covering `FindReferences`, `PrepareRename`, and `Rename` against the same "unresolved receiver, same-name unrelated symbol" fixture used by `unresolved_receiver_test.go`.

---

## Low / performance

### L11 — `GetWorkspaceSymbols` still uses literal `"builtin"` URI check
- **Where:** `internal/analyzer/analyzer.go:1028`.
- **What:** Workspace symbols listing still filters with `sym.URI == "" || sym.URI == "builtin"`, which misses stub-loaded builtins whose URIs are `builtin://...`. Those leak into workspace-symbol results.
- **Fix:** Replace with `sym.URI == "" || symbols.IsBuiltin(sym)` to match the rest of the analyzer post-Wave 1.

### L12 — `PrepareRename` / `Rename` check `Source` directly instead of using `IsBuiltin`
- **Where:** `internal/analyzer/analyzer.go:1233`, `:1336`.
- **What:** Functionally correct (`sym.Source == symbols.SourceBuiltin`) but inconsistent with the new `symbols.IsBuiltin` helper used everywhere else.
- **Fix:** Swap in `symbols.IsBuiltin(sym)`. Cosmetic.

### L13 — `phpdetect.Detect` does not expose a timeout sentinel
- **Where:** `internal/phpdetect/phpdetect.go`.
- **What:** `errors.Is(err, context.DeadlineExceeded)` does not compose with the error returned by `cmd.Output()` after a deadline (varies by OS; macOS interacts with `WaitDelay`). Callers cannot distinguish "timed out" from "binary missing" or "bad output".
- **Fix:** Export `var ErrTimeout = errors.New("phpdetect: timed out")` and wrap the returned error with it when the context deadline fires. Only useful once a caller surfaces the cause.

### L14 — Hand-authored availability entries overlap with the generated table
- **Where:** `internal/symbols/builtins.go` (`builtinAvailabilityByName`) vs `internal/symbols/builtin_availability_generated.go` (`generatedBuiltinAvailability`).
- **What:** `str_contains`, `str_starts_with`, `str_ends_with`, `json_validate`, `Fiber`, `WeakMap` are present in both with matching values. No correctness problem (precedence is well-defined and values agree), but the hand-authored entries become redundant once the real generator produces the comprehensive table.
- **Fix:** After the real `phpstorm-stubs` generator run lands (license review pending), prune redundant hand-authored entries. Keep only ones that genuinely need to override the generator.

### L15 — `generate-builtins.php availability` collects `@removed` but Go struct has no `Removed` field
- **Where:** `scripts/generate-builtins.php` (`generateGoAvailability`).
- **What:** PHP-side traversal parses `@removed X.Y` from PHPDoc but silently drops the value because `BuiltinAvailability` has no `Removed` field. Dead-end collection code; doc-rot risk.
- **Fix:** Either add a `Removed string` field to `BuiltinAvailability` (and route it into a future "symbol was removed in PHP >= X" diagnostic) or drop the `@removed` branch until the struct catches up.

### L16 — `builtin-unavailable` method signature deltas scope deferred
- **Where:** future `internal/checks/builtin_unavailable.go`.
- **What:** Detecting calls that match a newer overload than the project's PHP profile allows (e.g. `DateTimeImmutable::createFromInterface()` on PHP < 8.0) requires richer per-version stub layering than the current `internal/stubs/php/` provides.
- **Fix:** Land once per-version stubs are richer; gate behind `checks.builtin_unavailable.signature_deltas` defaulting off.

### L19 — Protocol-harness test for diagnostics re-publish is deferred
- **Where:** `internal/lsp/protocol_harness_test.go`.
- **What:** The harness drives requests synchronously through a pipe and has no mechanism to wait deterministically for the `postIndexSettle` goroutine. Asserting "first publish is soft-moded, second publish carries unknown-* findings" would require a settle channel or injectable goroutine. Skipped during U5; the soft-mode and re-publish are covered by package-level tests in `internal/symbols/` and `internal/diagnostics/`.
- **Fix:** Add a `WaitForSettle()` (or test-only callback) wired to the existing `indexWg` so harness tests can synchronize without sleeps.

### L17 — `.tusk-php.json` project-level config override
- **Where:** future config loader in `internal/config/` + resolution in `internal/lsp/server.go`.
- **What:** `BUILTIN_STUBS_PLAN.md` lists `.tusk-php.json` as step 3 of the PHP-version resolution chain (between composer `require.php` and local-PHP detection). Not implemented; the chain currently skips that step and proceeds straight from composer to `phpdetect`.
- **Fix:** Add a loader for `.tusk-php.json` at workspace root with `phpVersion` and optional `extensions` fields. Slot it into `resolveBuiltinProfile()`.

---

## Resolved

### L18 — PHP 8 attribute syntax (`#[Name]`) now visible to diagnostics
- `internal/checks/unknown_symbols.go::maskPHPLine` now leaves `#[` intact while still masking the historical `#`-to-end-of-line comment form.
- Added `unknownClassAttributeRe` plus `checkUnknownAttributeClasses` so `UnknownClassRule` emits an `unknown-class` finding (`Unknown attribute '…'`) for unknown attributes.
- Mirrored the same scope on `BuiltinUnavailableRule.checkClassLikesAttribute` so attributes like `#[Override]` on PHP 8.2 emit `Attribute 'Override' requires PHP >= 8.3`.
- Un-skipped `TestBuiltinUnavailableAttribute` and added `TestUnknownClassUnknownAttribute`.

### L20 — `phpdetect` timeout is now configurable, default raised to 1 s
- `phpdetect.Detect` now takes an explicit `timeout time.Duration`; `timeout <= 0` falls back to the exported `DefaultTimeout = 1 * time.Second`.
- Added `config.Config.PHPDetectTimeoutMs` for project-level override; `internal/lsp/server.go::resolvePHPProfile` reads it and propagates it to `Detect`.
- Tests now run against `DefaultTimeout` (production value) for happy paths and a deliberately short 200 ms for the sleep-based timeout test; `TestDetectDefaultsTimeoutWhenZero` exercises the sentinel.

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

### M5 — Parser diagnostics are now published from parser recovery state
- `internal/diagnostics/provider.go` now converts `FileNode.Errors` into `syntax-error`
  diagnostics (`SeverityError`, source `tusk-php`) during the fast analysis pass, so tokenizer
  and structural recovery failures are editor-visible through `publishDiagnostics`.
- `internal/parser/parser.go` now reports unterminated block comments at their opening `/*`
  location instead of the scan end, keeping parser and diagnostic coordinates stable.
- Regression coverage:
  `internal/diagnostics/provider_test.go`, `internal/lsp/coverage_test.go`,
  `internal/parser/parser_test.go`.

### M6 — Scope collector now models arrow captures and grouped imports
- `internal/scope/collector.go` now derives grouped top-level `use` aliases from the token stream
  and represents implicit `fn (...) => ...` captures as `BindingClosureUse` bindings linked to
  their outer origin.
- Regression coverage: `internal/scope/collector_test.go`.

### M7 — CursorContext scope only modeled enclosing class/method, not full position scope
- Fixed in `internal/source/context.go`: `Analyze` now derives cursor scope from
  `parser.ParseResult` ranges and token scanning, so `CursorContext.Scope` covers named free
  functions, anonymous `function (...) { ... }` closures, and class/trait/enum methods while
  preserving class FQN metadata for method/closure contexts.
- Regression coverage: `internal/source/context_test.go`.

### M8 — CursorContext namespace/import state was file-global rather than position-aware
- Fixed in `internal/source/context.go`: `Analyze` now derives the active namespace block and
  import list from the token stream at the cursor position, so multiple namespace blocks and
  later import regions no longer leak into earlier positions.
- Regression coverage: `internal/source/context_test.go`.

### M9 — `list(...)` destructuring is now collected
- `internal/scope/collector.go` now treats `list(...) = ...` as a destructuring declaration form,
  sharing the collector binding surface used by references and rename.
- Regression coverage: `internal/scope/collector_test.go`.

### M10 — `PrepareRename` now requires a scoped variable binding
- `internal/analyzer/analyzer.go` now routes variable prepare-rename checks through
  `scope.Collect(source).BindingAt(pos)` and only advertises rename for supported local binding
  kinds that the rename path can edit.
- Regression coverage: `internal/analyzer/rename_test.go`.

### M11 — CursorContext now models arrow-function scope
- `internal/source/context.go` now recognizes `fn (...) => ...` expression bodies as closure-like
  cursor scopes, so positions inside arrow functions no longer inherit only the surrounding
  function or method scope.
- Regression coverage: `internal/source/context_test.go`.

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

### L9 — Interface implementation lookup now includes enums
- Enum indexing now records each resolved implemented interface in `reverseImplementsMap`, and
  analyzer implementation results allow enum symbols alongside classes. Regression coverage in
  `internal/analyzer/navigation_test.go`, `internal/lsp/coverage_test.go`, and
  `internal/symbols/index_test.go`.

### L10 — Sprint 3 unknown diagnostics are now wired natively
- `internal/checks/unknown_symbols.go` adds configurable `unknown-class`, `unknown-function`,
  and `unknown-member` checks over the compatibility AST plus local in-file declarations.
- `internal/diagnostics/provider.go` now runs those checks during the fast analysis pass, and the
  provider/LSP regression tests cover the native code paths in
  `internal/diagnostics/provider_test.go` and `internal/lsp/coverage_test.go`.
