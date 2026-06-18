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

_None open._

---

## Low / performance

### L32 — `lockfile.toLocked` discards the transitive `require` map
- **Where:** `internal/composer/lockfile/lockfile.go` (`toLocked`, `LockedPackage`); only `Find(name)` is exposed, no iterate-all accessor.
- **What:** `composer.lock` packages carry a full `require` map (package→package edges), but `toLocked` keeps only `Require["php"]` as `PHPRequire` and drops the rest. The dependency-tree map in the planned `deps tree` command (see `docs/dependency-usage-analysis-design.md`) needs these edges plus a way to iterate all locked packages.
- **Fix:** Add a `Require map[string]string` field to `LockedPackage`, carry it through `toLocked`, and add an iterate-all accessor (and dev/non-dev distinction if needed). Cheap — data is already parsed into `rawPackage.Require`.

### L31 — No per-package exposed-surface count for dependency usage-%
- **Where:** `internal/composer` + `internal/symbols`; consumed by the planned `deps usage` command (`docs/dependency-usage-analysis-design.md`).
- **What:** Usage-% needs a denominator = distinct public classes a package exposes under its `autoload` PSR-4 roots (excluding `autoload-dev`). Vendor symbols are indexed lazily/by-need (`SourceVendor`), so the index may not hold a package's full public surface, making a naive count under-report.
- **Fix:** Add a briefing-grade per-package surface count — either a dedicated "enumerate autoload roots" pass over `install-path` + PSR-4 prefixes, or a flag to fully index a package's autoload roots on demand. Briefing-grade approximation is acceptable for v1.

### L40 — `InterfaceNode`/`TraitNode`/`EnumNode`/`FunctionNode` (compat layer) lack `EndLine`
- **Where:** `internal/parser/compat.go` (struct defs ~80-128; `toFileNode` conversion ~550-617).
- **What:** Same gap L23 fixed for `ClassNode`: the compat nodes for interfaces, traits, enums, and functions don't expose `EndLine`, although `InterfaceDef`/`TraitDef`/`EnumDef`/`FunctionDef` all carry it. (Discovered while fixing L23.)
- **Fix:** Add `EndLine` to those four compat node types and map it through `toFileNode`, mirroring the `ClassNode` fix.

### ~~L30~~ — `phpdetect` tests flake under parallel load — **RESOLVED**
- Happy-path tests now use a generous `testDetectTimeout = 5s` instead of the production `DefaultTimeout`; the dedicated timeout-assertion tests keep their short 200ms value. Stable under `-count=3`.

### L29 — `php_graph` MCP tool wraps mermaid/dot output in JSON instead of raw text
- **Where:** `internal/mcpserver/server.go` (`phpGraph`, ws/mcp-maturation).
- **What:** mermaid/dot formats return `{"format": ..., "text": ...}` structured content because `mcp.Content` in go-sdk v1.6.1 could not be constructed directly. Functional, but agents must unwrap the `text` field.
- **Fix:** Revisit when the SDK exposes a text-content constructor; emit raw text content for non-JSON formats.

### ~~L28~~ — `laravel_*` MCP tools are not framework-gated — **RESOLVED**
- `laravel_routes` / `laravel_route_to_controller` / `laravel_model_schema` registration is now gated on `s.Framework == "laravel"` (mirroring the Symfony gate); covered by `TestLaravelToolsVisible`.

### ~~L27~~ — `parseParams` variadic detection is dead code — **RESOLVED**
- `parseParams` now detects the three-consecutive-`TokenUnknown "."` sequence (the real tokenization of `...`) and sets `ParamDef.IsVariadic` for both `...$x` and `Type ...$x`. Covered by `TestParseVariadicParams`.

### L26 — `graph models` misses Doctrine XML mappings and string-form targetEntity
- **Where:** `internal/models/relations.go` (`ModelRelations` Symfony path); relies on `ormRelationRe` + `targetEntityRe` (doctrine.go:52,67).
- **What:** Doctrine relation edges are only derived from PHP 8 attribute syntax with `targetEntity: Foo::class`. Two gaps: (1) entities mapped purely via XML (`config/doctrine/*.orm.xml`) emit no edges, though `doctrine.go` already parses that XML (`parseDoctrineXMLMapping`); (2) string-form targets (`targetEntity: 'App\Entity\Foo'`) aren't matched by `targetEntityRe`. Both produce missing edges (under-reporting) in `graph models` on Symfony/Doctrine projects.
- **Fix:** Add an XML-relation path to the Symfony branch of `ModelRelations` (reuse the existing XML parse), and extend `targetEntityRe` to also accept quoted string targets.

### L25 — Symfony `bundles.php` exclusion not implemented for unused-deps
- **Where:** `internal/unuseddeps/unuseddeps.go` (`Analyze`); analogous to `composer.LaravelAutoDiscoveredPackages`.
- **What:** Unused-dependency analysis excludes Laravel auto-discovered packages (`extra.laravel.providers`/`aliases`) but has no Symfony equivalent. Symfony bundles registered in `config/bundles.php` are used via framework wiring, not static code refs, so their packages will be reported as unused-candidates (false positives) on Symfony projects.
- **Fix:** Add `composer.SymfonyBundlePackages(rootPath)` (parse `config/bundles.php` bundle classes → owning packages via the package resolver) and union it into the `excluded` set in `Analyze`. The exclusion set is already structured additively.

### L24 — `resolve.ResolveClassName` may return a bare short name when the FQN isn't indexed
- **Where:** `internal/resolve/resolve.go` (`ResolveClassName`, ~line 106); affects `new X` edges in `internal/graph/references.go`.
- **What:** When a `new X` target's namespace-qualified FQN isn't yet in the index, `ResolveClassName` falls back to the bare short name. In `graph references --deps full` this can surface as an orphan node with an unqualified ID. `DepsNone`/`DepsBoundary` are unaffected (such nodes classify as "unresolved" and are handled).
- **Fix:** Have the references builder prefer the namespace-qualified candidate for unresolved `new` targets, or drop targets that don't resolve to an indexed symbol under DepsFull.

### ~~L23~~ — `ClassNode` (compat layer) does not expose `EndLine` — **RESOLVED**
- `ClassNode` gained an `EndLine` field, mapped from `classDef.EndLine` in `toFileNode`. Covered by `TestClassNodeEndLine`. (The same gap for interface/trait/enum/function nodes is tracked in L40.)

### ~~L21~~ — No test seam to inject container bindings, leaving DepsBoundary Meta path conditionally covered — **RESOLVED**
- Added exported `(*ContainerAnalyzer).AddBinding(*ServiceBinding)`; `graph/container_test.go` now injects a vendor-classified binding directly and asserts the `DepsBoundary` Meta (`edgeCount`/`distinctSymbols`/`version`) unconditionally (the `t.Skip` escape hatch is gone).

### ~~L20~~ — Duplicate installed.json unmarshal structs in composer package — **RESOLVED**
- `installedPackage` in `composer.go` now includes `Version string \`json:"version"\``.
  `installedPackageWithVersion` and `installedJSONWithVersion` removed from `packages.go`;
  `InstalledPackages` repoints at the unified struct. Behaviour is identical.

### L14 — Hand-authored availability entries overlap with the generated table
- **Where:** `internal/symbols/builtins.go` (`builtinAvailabilityByName`) vs `internal/symbols/builtin_availability_generated.go` (`generatedBuiltinAvailability`).
- **What:** `str_contains`, `str_starts_with`, `str_ends_with`, `json_validate`, `Fiber`, `WeakMap` are present in both with matching values. No correctness problem (precedence is well-defined and values agree), but the hand-authored entries become redundant once the real generator produces the comprehensive table.
- **Fix:** After the real `phpstorm-stubs` generator run lands (license review pending), prune redundant hand-authored entries. Keep only ones that genuinely need to override the generator.

### L16 — `builtin-unavailable` method signature deltas scope deferred
- **Where:** future `internal/checks/builtin_unavailable.go`.
- **What:** Detecting calls that match a newer overload than the project's PHP profile allows (e.g. `DateTimeImmutable::createFromInterface()` on PHP < 8.0) requires richer per-version stub layering than the current `internal/stubs/php/` provides.
- **Fix:** Land once per-version stubs are richer; gate behind `checks.builtin_unavailable.signature_deltas` defaulting off.

### L19 — Protocol-harness test for diagnostics re-publish is deferred
- **Where:** `internal/lsp/protocol_harness_test.go`.
- **What:** The harness drives requests synchronously through a pipe and has no mechanism to wait deterministically for the `postIndexSettle` goroutine. Asserting "first publish is soft-moded, second publish carries unknown-* findings" would require a settle channel or injectable goroutine. Skipped during U5; the soft-mode and re-publish are covered by package-level tests in `internal/symbols/` and `internal/diagnostics/`.
- **Fix:** Add a `WaitForSettle()` (or test-only callback) wired to the existing `indexWg` so harness tests can synchronize without sleeps.

### ~~L17~~ — `.tusk-php.json` project-level config override — **RESOLVED**
- `config.Config` gains `Extensions []string` and an unexported `phpVersionExplicitlySet` flag
  (exposed via `IsPHPVersionExplicitlySet()`). `LoadFromFile` detects the presence of the
  `phpVersion` JSON key; `MergeClientOptions` sets the flag when `PHPVersion` is non-empty.
- `workspace.ResolveBuiltinProfileWithConfig` implements the four-step chain:
  (1) composer `config.platform.php` / `require.php`, (2) config `phpVersion` when explicitly
  set, (3) phpdetect, (4) bundled default. Config `extensions` are merged additively with
  composer extensions. `server.go` now calls `ResolveBuiltinProfileWithConfig` passing `s.cfg`.

---

## Resolved

### M12 — `builtin-unavailable` constants scope implemented
- `internal/checks/builtin_unavailable.go` adds `checkConstants` with per-line scanning that skips
  class constants (`Foo::CONST`), function calls (`name(`), and `const NAME` declaration contexts.
  Bitwise-or chains are checked per-token so partially-unavailable chains flag only the unavailable
  identifier.
- `internal/symbols/builtins.go` seeds 10 well-known version-gated constants: `JSON_THROW_ON_ERROR`
  (7.3), `T_FN` / `ARRAY_FILTER_USE_BOTH` (7.4), `FILTER_VALIDATE_BOOL` / `T_NAME_QUALIFIED` /
  `T_NAME_FULLY_QUALIFIED` / `T_NAME_RELATIVE` (8.0), `MYSQLI_REFRESH_REPLICA` / `CURLOPT_DOH_URL`
  (8.1), `SEEK_HOLE` / `SEEK_DATA` (8.3).
- Regression coverage in `internal/checks/builtin_unavailable_test.go`:
  `TestBuiltinUnavailableConstant`, `TestBuiltinUnavailableConstantBitwiseOr`,
  `TestBuiltinUnavailableConstantClassConstantNotConfused`,
  `TestBuiltinUnavailableConstantOnSufficientVersion`.

### M13 — `resolveSymbolAtCursor` no longer falls back to `LookupByName` in `->`/`::` context
- W1 guard mirrored into `resolveSymbolAtCursor` in `internal/analyzer/analyzer.go`: when
  `ctx.AccessKind != AccessNone`, the function returns `nil` at the end of the access-chain branch
  instead of falling through to direct-lookup / `LookupByName` fallbacks that could match unrelated
  same-name symbols across the project.
- Companion guards added to `FindAllReferences` (post-`resolveSymbolAtCursor` nil check) and
  `PrepareRename` (the unresolvable `AccessKind != AccessNone` branch now returns `nil` instead of
  advertising a rename target).
- Regression coverage in `internal/analyzer/unresolved_receiver_test.go`:
  `TestFindReferencesUnresolvedReceiverNoFalsePositive`,
  `TestPrepareRenameUnresolvedReceiverNoFalsePositive`,
  `TestRenameUnresolvedReceiverNoFalsePositive`.

### L11 — `GetWorkspaceSymbols` now uses `IsBuiltin` helper
- `internal/analyzer/analyzer.go::GetWorkspaceSymbols` filter changed from `sym.URI == "builtin"`
  to `symbols.IsBuiltin(sym)`, so stub-loaded builtins with `builtin://...` URIs no longer leak
  into workspace-symbol results.

### L12 — `PrepareRename` / `Rename` now use `IsBuiltin` helper
- `internal/analyzer/analyzer.go::PrepareRename` swaps `sym.Source == symbols.SourceBuiltin` for
  `symbols.IsBuiltin(sym)`; `Rename` replaces the SourceBuiltin half of its guard with
  `symbols.IsBuiltin(sym)` while preserving the existing SourceVendor check.

### L13 — `phpdetect.Detect` now exposes `ErrTimeout` sentinel
- `internal/phpdetect/phpdetect.go` exports `var ErrTimeout = errors.New("phpdetect: timed out")`.
  After `cmd.Output()` returns an error, if `errors.Is(ctx.Err(), context.DeadlineExceeded)` is
  true the error is wrapped so that `errors.Is(err, phpdetect.ErrTimeout)` composes correctly
  across OSes.
- New test `TestDetectTimesOutWrapsErrTimeout` in `internal/phpdetect/phpdetect_test.go`;
  `TestDetectEmptyBinaryPathDefaultsToPath` extended to also accept `ErrTimeout` as a valid error
  kind (a slow PATH-resolved `php` now surfaces as a timeout rather than as a missing binary).

### L15 — `generate-builtins.php availability` no longer collects `@removed`
- `scripts/generate-builtins.php` drops `parseRemovedFromDocComment` and the `Removed` entry
  assembly path. The `@since` parsing path is unchanged. Will be re-added if/when
  `symbols.BuiltinAvailability` gains a `Removed` field for a "symbol removed in PHP >= X"
  diagnostic.

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
