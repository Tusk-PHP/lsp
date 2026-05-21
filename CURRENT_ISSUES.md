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

### M13 — Inline variable still lacks safe single-assignment and side-effect analysis
- **Where:** `internal/analyzer`, `internal/scope`
- **What:** Sprint 7 now offers a conservative extract-variable refactor for single-line expression selections in local brace-backed scopes, but inline-variable is still unsafe because the current scope model does not prove a binding has exactly one dominating assignment or that substituting the initializer preserves precedence and side effects across all uses.
- **Fix:** Extend the scope/refactor pipeline with statement-aware assignment tracking, dominance checks for local reassignments, and expression-parenthesization rules, then add inline-variable code actions only for bindings that satisfy those constraints.

### M12 — Symfony PHP service config files are not analyzed
- **Where:** `internal/container/analyzer.go`
- **What:** Symfony container discovery only reads `config/services.yaml|yml` plus class/interface autowiring from `src/`; projects using PHP DI config (`config/services.php`, package PHP config files) do not surface those service IDs or aliases.
- **Fix:** Add a focused extractor for `ContainerConfigurator` / fluent `->set()` / `->alias()` service definitions in Symfony PHP config files and cover it with container completion/definition tests.

---

## Low / performance

### L12 — Code action kind filtering is duplicated across analyzer helpers
- **Where:** `internal/analyzer/import_code_actions.go`, `internal/analyzer/unknown_class_code_actions.go`
- **What:** `supportsCodeActionKind` and `importCodeActionKindAllowed` implement the same prefix-matching logic separately, which makes future refactor action additions easy to wire inconsistently.
- **Fix:** Consolidate code-action kind matching behind one shared analyzer helper and reuse it across quickfix/source/refactor providers.

### L11 — Symfony route discovery only covers attribute routes in indexed PHP sources
- **Where:** `internal/framework/symfony/routes.go`
- **What:** Sprint 6 route-name support currently discovers `#[Route(...)]` declarations from project PHP files, but does not yet parse `config/routes.{yaml,yml,xml}` files or imported route resources, so some valid Symfony route names will be missing from completion/definition.
- **Fix:** Add a Symfony route loader that parses route config files and import prefixes, then merge those results with attribute-derived routes in the shared framework route index.

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
