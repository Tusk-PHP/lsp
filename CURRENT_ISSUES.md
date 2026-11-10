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

### L62 — A call made only for another version of a library is reported as having too few arguments
- **Where:** `argument-count-mismatch` in `internal/checks/argument_count.go`.
- **What:** Laravel's `Foundation/Bootstrap/HandleExceptions.php` (line 359) calls `$instance->enable()` in the `else` of `if (version_compare(Version::id(), '12.3.4', '>='))`, for PHPUnit versions before the one installed, whose `enable()` takes a `TestCase`. Symfony's `Bridge/Twig/Form/TwigRendererEngine.php` (line 242) calls `->unwrap()`, which the installed Twig 4 gives a required `Environment` parameter. Both are correct for the installed versions, but the code supports others.
- **Fix:** Stay quiet inside a branch guarded by `version_compare()`, and weigh the project's `composer.json` constraints when the installed version is newer than what it allows.

### L61 — Placeholder classes shadow the real ones when a variable is narrowed to them
- **Where:** the symbol index (`internal/symbols`), seen through `instanceof` narrowing in `internal/resolve/instanceof.go`.
- **What:** Symfony's `Bridge/PhpUnit/SymfonyTestsListener.php` declares an empty `SymfonyTestsListener` inside `if (false)` next to a `class_alias()` to the real class, and its tests ship `stubs/mongodb.php` with bare `MongoDB\Collection`, `Database` and `Client` classes. The index takes those as the classes, so `if ($l instanceof SymfonyTestsListener) { $l->globalListenerDisabled(); }` (`Legacy/CommandForV8.php`, `CommandForV9.php`) and `$mongo->withOptions()` and `->getCollection()` on `MongoDB\Collection`, `Database` and `Client` in `Cache/Traits/MongoDbTrait.php` are reported as unknown members. Before `instanceof` narrowing these variables had no known type, so nothing was reported. A parameter declared with those classes is reported the same way.
- **Fix:** Index a class declared under `if (false)` as a placeholder that loses to the `class_alias()` target, and leave test stub folders (`Tests/**/stubs`) out of the index or below vendor code.

### L60 — Hover and go-to-definition find nothing on a member of a called closure's result
- **Where:** member hover in `internal/hover/provider.go`, member definition in `internal/analyzer/analyzer.go` (their fallback chain walkers, see L49).
- **What:** With `$g = fn (int $i): Foo => new Foo();`, `$x = $g(1);` types `$x` as `Foo`, but hover and go-to-definition on `fooOnly` in `$g(1)->fooOnly()` find nothing, and `$g(1)->nope()` isn't reported as an unknown member on `Foo`. The same on 0.13.0.
- **Fix:** Route both through `resolve`'s chain resolution, which knows what calling a variable gives.

### L59 — A variable destructured into keeps the type it had before
- **Where:** `reachingType` in `internal/resolve/reaching.go` (the fallback to the line scan); `instanceofOwners` in `internal/checks/unknown_symbols.go`.
- **What:** In Symfony's `Routing/Matcher/Dumper/CompiledUrlMatcherDumper.php` (line 405), `foreach ($routes as $route) { if ($route instanceof StaticPrefixCollection) { …; continue; } [$name, $regex, $vars, $route] = $route; $route->compile(); }` reports `Unknown member 'compile' on 'StaticPrefixCollection'`. The only write that reaches the call is the destructuring, which can't be typed on its own, so the old line scan answers, finds the `foreach` line and gives the loop's element type; the `instanceof` owner from the block that ended in `continue` is added on top. The same on 0.13.0.
- **Fix:** Mark destructuring writes in the collector and return unknown for them instead of falling back to the line scan. Separately, drop an `instanceof` owner once the variable is written again or the tested block leaves with `continue`, `break`, `return` or `throw`.

### L58 — Hover and go-to-definition on a member of a union receiver look at one class
- **Where:** member hover in `internal/hover/provider.go`, member definition in `internal/analyzer/analyzer.go`.
- **What:** After `if ($c) { $x = new Foo(); } else { $x = new Bar(); }`, `$x->barOnly()` is accepted and completed, but hover and go-to-definition on `barOnly` find nothing: they look the member up on one class of `Foo|Bar`. The same on 0.13.0 for any union receiver; branch-assigned variables now make it more common.
- **Fix:** Look the member up on each class of the receiver's `MemberOwner()`, as completion and `unknown-member` do.

### L57 — Branch shapes that still give a wider union than they should
- **Where:** `internal/scope/reach.go` (`ReachingWrites`), `internal/scope/flow.go`.
- **What:** A branch that ends in `return` or `throw` still reaches the code after it (`if ($c) { $h = new Bar(); return; }` leaves `Bar` in the union), and a value written before a `switch` whose every `case` and `default` assigns stays in the union (`Box<int>|Foo|Bar`). Wider, never wrong, so nothing false is reported; a member only the dropped class has is accepted.
- **Fix:** Record blocks that end in `return`/`throw`/`continue`/`break` and leave their writes out; treat a `switch` with a `default` where every arm assigns like a covered `if`/`else` chain.

### L56 — A read that more than 32 writes may reach is left unknown
- **Where:** `scope.MaxReachingWrites` in `internal/scope/reach.go`.
- **What:** The cap keeps a read's cost bounded (a file with 1,500 conditional writes opens in 70 ms instead of 2.5 s). Past it the variable has no type: no hover type, no completion, no member checks. How often real code reaches the cap hasn't been measured; a long generated `switch` would.
- **Fix:** If it shows up in real code, stop at the cap with the union of the distinct types seen when every write examined so far typed to the same few classes, instead of giving up.

### L55 — Values that still can't be typed make a branch-assigned variable unknown
- **Where:** `internal/resolve/expression.go` (`ExpressionType`), `internal/resolve/resolve.go` (`resolveVariableTypeTypedBase`), `internal/symbols/builtin_registry_generated.go`.
- **What:** A variable is now unknown when any value that can reach it is. That exposes values nothing types: ternaries, `$a ?? $b`, arithmetic and array `+`, an offset into an untyped array, `static $x;` without a value (so `static $memo; $memo ??= new Foo();` went from `Foo` to unknown), `$x .= …` (not tracked as a string write), and builtins the generated registry lists by name only (`addslashes`). `str_replace` returns `string|array` without its conditional return type. In a 12-file WordPress sample 405 of 17,545 variable hovers lost a type this way (1,042 gained one); in Laravel 35 of 3,894 (79 gained).
- **Fix:** Type them in `ExpressionType`, in this order of payoff: ternary and `??` (union of both sides), arithmetic, then return types for the registry's bare builtins.

### L54 — An assignment inside an expression is still answered by the line scan
- **Where:** `variableScope.assignmentValue` in `internal/resolve/variable_scope.go`; `reachingType` in `internal/resolve/reaching.go`.
- **What:** `foo($x = new Bar())` and `for ($i = 0; …)` aren't whole statements, so their value isn't read at the write. The old backward line scan answers and can give the value of an earlier line, as on 0.13.0. Multi-line values also still go through `continuedExpression`, which reads raw lines.
- **Fix:** Read a write's value from its token range (`WriteSite.RHS` to `End`) for every write, which also retires `continuedExpression`.

### L53 — The resolution guard is keyed by goroutine id
- **Where:** `currentGID`, `enterGuard` in `internal/resolve/resolve.go`.
- **What:** Root `AGENTS.md` rules out goroutine-identity tricks, and the cycle guard for type resolution finds its per-request state that way. It predates the false-warning fixes; the branch-values work adds one more caller of the same guard because the cycle runs through completion's chain-resolver callbacks, which carry no request state.
- **Fix:** Give resolution a request struct (or `context.Context`) that carries the guard, and pass it through the chain-resolver callbacks.

### L52 — `ResolveVariableType` returns a string that each caller parses again
- **Where:** `Resolver.ResolveVariableType` and `plainTypeName` in `internal/resolve`; callers in `internal/analyzer/analyzer.go` (`definitionForVariable`, `locationsForTypeName`), `internal/completion/provider.go`, `internal/hover/provider.go`.
- **What:** The plain resolver gives a class name, or `A|B|null` for several values, and callers split or trim it themselves. Returning a fuller string there broke go-to-definition on a variable during the branch-values work until a test caught it.
- **Fix:** Move the callers to `ResolveVariableTypeTyped` and `MemberOwner()`/`ClassNames()`, then drop the plain form (root `AGENTS.md`: structured types).

### L51 — A comment between `->` and the member name makes the call an unknown function
- **Where:** the `unknown-function` rule in `internal/checks/unknown_symbols.go`.
- **What:** `$f->/* note */bindTo(null);` reports `Unknown function 'bindTo'`: the rule looks at the text before the name and finds the comment, not the `->`. Valid PHP, rare in real code; the same on 0.13.0.
- **Fix:** Decide "is this a member call" from the previous significant token, as the shared tokens give it.

### L50 — `completion/provider.go` and `resolve/resolve.go` are well past the 800-line guideline
- **Where:** `internal/completion/provider.go` (1,564 lines), `internal/resolve/resolve.go` (1,459 lines), `internal/scope/collector.go` (1,539 lines).
- **What:** All three were already over the guideline in root `AGENTS.md` before the false-warning fixes, which added only call-site lines to them (new logic went to `scope/flow.go`, `scope/reach.go`, `resolve/reaching.go`, `resolve/concat.go`). Every new typing rule lands in the same two functions (`resolveChainWithContext`, `resolveVariableTypeTypedBase`).
- **Fix:** Split within the package before the next feature there: the chain walkers out of `provider.go`, the assignment branches of `resolveVariableTypeTypedBase` into one file per right-hand-side form.

### L49 — Hover's and the analyzer's fallback chain walkers read `f(...)` as a call
- **Where:** `internal/hover/provider.go` (`resolveAccessChainTyped`, line 311); `internal/analyzer/analyzer.go` (`resolveAccessChainTyped`, about line 532).
- **What:** First-class callable syntax is a `Closure` in the shared resolver and in completion's walkers, but these two copies still type `$obj->m(...)` as what `m()` returns. They run only when the shared resolver returns nothing, so it isn't visible today.
- **Fix:** Route both through `resolve.IsFirstClassCallableArgs`, or remove the copies once `session` wires one resolver for every feature.

### L48 — `unknown-member` stays quiet on a receiver typed `Closure(…): T`
- **Where:** `internal/checks/unknown_symbols.go` (`collectClassLikes`, line 706).
- **What:** There is no branch for the callable type kind, so a closure with a signature (written, or from `f(...)`) reads as an unknown owner and `$closure->fooOnly()` is not reported. Safe, but a real error is missed. A plain `Closure` is checked, now that the stubs declare its members.
- **Fix:** Treat a callable kind named `Closure` as the class `Closure`; leave `callable(…)` alone.

### L47 — Laravel notification methods called by convention are still reported
- **Where:** `Illuminate/Notifications/Channels/MailChannel.php:55` (`toMail`), `Illuminate/Notifications/NotificationSender.php:233` (`via`) in the Laravel field run.
- **What:** `Notification` declares neither method; channels call them by convention with no `method_exists()` guard, so `unknown-member` fires. The guard rule correctly doesn't cover them.
- **Fix:** Teach `framework/laravel` the notification channel methods (`via`, `toMail`, `toArray`, `toDatabase`, `toBroadcast`, …) as known on `Notification` subclasses; not in the generic check.

### L46 — Existence guards written over several lines, or in upper case, aren't recognised
- **Where:** `internal/checks/member_guards.go` (`memberGuardCallRe`, `memberGuards.add`).
- **What:** `method_exists(` with its arguments on following lines, and `METHOD_EXISTS(…)`, don't register as guards, so the guarded member is still reported. It fails towards warning, not towards silence.
- **Fix:** Read the guard call from the shared tokens instead of one masked line; compare the function name case-insensitively.

### L45 — A nullsafe link cuts the receiver of a longer chain
- **Where:** `internal/checks/unknown_symbols.go` (`isExprChar`, line 1535).
- **What:** `?` isn't an expression character, so the receiver of `$a?->b->c()` is read as `->b` onwards and the member check (and the guard match) sees the wrong owner expression.
- **Fix:** Accept `?` when it is followed by `->`, with a test for `$a?->b->c()` and for a ternary that ends just before a variable.

### L44 — 17 files under `internal/` aren't gofmt-clean
- **Where:** `gofmt -l internal/`, for example `internal/config/config.go`, `internal/mcpserver/server.go`, `internal/workspace/workspace.go`.
- **What:** Pre-existing on 0.13.0; unrelated diffs pick up formatting noise when an editor formats on save.
- **Fix:** One `chore: gofmt` commit, and `gofmt -l` as a CI step.

### L43 — Tests that need `testdata/laravel/vendor` fail instead of skipping
- **Where:** `internal/analyzer/container_test.go` (`TestDefinitionAppStringBinding`, `TestDefinitionAppChainedMethod`, `TestDefinitionResolveHelper`); 15 end-to-end tests in `internal/completion`; `TestE2ESymfonyCompletions` in `internal/lsp` with an incomplete `testdata/symfony/vendor`.
- **What:** `testdata/laravel/vendor` is git-ignored, so in a fresh clone or worktree these fail with or without any change. They pass in a checkout where `composer install` has been run there.
- **Fix:** Skip with a message naming the setup step when the vendor directory is missing, and add that step to `make test` or the contributing notes.

### L42 — `unknown-class` before `::` is still matched with a regular expression per line
- **Where:** `internal/checks/unknown_symbols.go` (`unknownClassStaticRe`, line 21; `checkUnknownStaticClassMatches`).
- **What:** The fix for `$this->prop::method()` looks back for `->` on the line (and the previous line). A name that follows `::` or `)` and comes before another `::` still depends on the pattern alone (not probed for false warnings yet), against the "tokens, not raw lines" rule.
- **Fix:** Move the rule to the shared tokens; that needs the rule's `Check` to receive them.

### L41 — `laravel_model_schema` re-derives all project relations per single-model call
- **Where:** `internal/mcpserver/server.go` (`laravelModelSchema` handler); `internal/models/relations.go` (`ModelRelations`).
- **What:** The handler calls `models.ModelRelations` over the whole project and then filters to the requested model FQN — O(all project relations) per single-model query. Accepted per the graph-cli-removal plan ("revisit only if it shows up in profiling"); not a correctness issue.
- **Fix:** Add a per-model relation derivation (filter during iteration, or index relations by `SourceModel`) if profiling shows it matters.

### L40 — `InterfaceNode`/`TraitNode`/`EnumNode`/`FunctionNode` (compat layer) lack `EndLine`
- **Where:** `internal/parser/compat.go` (struct defs ~80-128; `toFileNode` conversion ~550-617).
- **What:** Same gap L23 fixed for `ClassNode`: the compat nodes for interfaces, traits, enums, and functions don't expose `EndLine`, although `InterfaceDef`/`TraitDef`/`EnumDef`/`FunctionDef` all carry it. (Discovered while fixing L23.)
- **Fix:** Add `EndLine` to those four compat node types and map it through `toFileNode`, mirroring the `ClassNode` fix.

### L39 — `checks.AllRules()` is stale dead code
- **Where:** `internal/checks/checks.go` (`AllRules`, ~line 58).
- **What:** `AllRules()` returns only three rules (UnusedImports/UnusedPrivate/UnreachableCode) and was never updated as new rules were added; it has zero production callers, so it silently misrepresents the rule set. (Discovered during the L34 unification.)
- **Fix:** Delete `AllRules()` — the canonical native set now lives in `diagnostics.NativeRules`. (`checks` cannot import `diagnostics`, so don't delegate; just remove.)

### ~~L38~~ — Document introspection dumps only the first top-level symbol — **RESOLVED**
- `introspect.Document` now builds a section for every top-level class/interface/trait/enum (`Dump.Symbols`), rendered with dividers; `PrimarySymbol` retained as the first element for back-compat. Cursor-first ordering deferred (no Position plumbing needed for "dump all").

### L37 — `introspect` CLI URI derivation: not a bug (corrected)
- **Where:** `cmd/tusk-php/main.go` (`runIntrospect`) vs `internal/workspace/workspace.go`.
- **What:** CORRECTION — `workspace` keys files as `"file://"+path` (raw OS path, no `ToSlash`), and `runIntrospect` uses the same `"file://"+absFile` convention, so the two MATCH on every OS and the dump works (verified on Unix). The earlier "empty dump on Windows" premise was wrong.
- **Fix:** No introspect-specific fix needed. The shared non-standard `file://` form on Windows (backslashes) is a pre-existing, workspace-wide concern; if proper RFC-8089 `file://` URIs are wanted, normalise in ONE shared path→URI helper used by both `workspace` and the CLI. Low-priority hygiene, not a correctness bug.

### ~~L36~~ — `tuskPhpLsp.debugDocument` lacks test coverage — **RESOLVED**
- `execute_command_test.go` now asserts `tuskPhpLsp.debugDocument` in the advertised set and adds `TestExecuteCommandDebugDocument` (round-trip: invokes the command, asserts a non-empty string containing the dump header + class name).

### ~~L35~~ — Introspection dump header omits the PHP profile string — **RESOLVED**
- Added `introspect.FormatProfile`; both callers (`server.go` debugDocument via `s.workspace.BuiltinProfile`, `main.go` runIntrospect via `ws.BuiltinProfile`) now populate `Input.Profile`. Header renders e.g. `Server  0.9.0 · PHP 8.3 · framework laravel`.

### ~~L34~~ — Native check-rule set duplicated across three call sites — **RESOLVED**
- Added `diagnostics.NativeRules(opts)` as the single constructor; `server.go` and `cmd/tusk-php/main.go` now call it (duplicate `nativeCheckRules` helpers removed). `provider.go` keeps its per-rule-severity model, guarded by `TestNativeRulesParityWithProvider` so the lists can't drift.

### L33 — `deps usage` exposed-class denominator under-counts (capped, not fixed); no lockfile fixture for `deps tree` on testdata/project
- **Where:** `internal/deps/usage.go` (`Usage` denominator pass); `testdata/project/` (no `composer.lock`); `internal/composer/packages.go` (`InstalledPackages` empty `Version` for stub vendors).
- **What:** Because vendor is indexed by-need, the per-package exposed-class count can fall below the touched count, which produced nonsensical >100% usage (observed `monolog/monolog 200.0% (2/1)` on `testdata/project`). It is now **clamped** by flooring the denominator at the touched count (Percent ∈ [0,100]), but the underlying denominator is still a briefing-grade under-count (the real fix is the L31 exhaustive-surface pass). Separately, `testdata/project` has no `composer.lock`, so `deps tree` cannot be exercised against that canonical fixture (only `testdata/laravel` works), and `InstalledPackages` returns an empty `Version` for the manually-placed monolog stub.
- **Fix:** Implement the exhaustive per-package surface count from L31 so the denominator is real; add a minimal `composer.lock` to `testdata/project` mirroring its `composer.json` so `deps tree` has a canonical fixture; optionally backfill `version` on the stub vendor or its `installed.json`.

### ~~L32~~ — `lockfile.toLocked` discards the transitive `require` map — **RESOLVED**
- `LockedPackage` now carries `Require map[string]string` (full package→package edges), and `(*Lockfile).All()` returns every locked package sorted by name. Consumed by `deps.BuildTree`.

### L31 — Exhaustive per-package exposed-surface count for dependency usage-% (briefing-grade shipped)
- **Where:** `internal/composer` + `internal/symbols`; consumed by `internal/deps/usage.go` (`deps usage`).
- **What:** Usage-% needs a denominator = distinct public classes a package exposes under its `autoload` PSR-4 roots (excluding `autoload-dev`). Vendor symbols are indexed lazily/by-need (`SourceVendor`), so the current `SearchByFQNPrefix`-based count under-reports (see L33). The briefing-grade approximation is **shipped and accepted for v1**; this entry tracks the exhaustive version.
- **Fix:** Add a dedicated "enumerate autoload roots" pass over `install-path` + PSR-4 prefixes (or fully index a package's autoload roots on demand) so the denominator reflects the real exposed surface, not just by-need-indexed classes.

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

### ~~L25~~ — Symfony `bundles.php` exclusion not implemented for unused-deps — **RESOLVED**
- `composer.SymfonyBundlePackages(rootPath)` now parses `config/bundles.php` → bundle class FQNs → owning packages via the resolver, and `unuseddeps.Analyze` unions them into the excluded set (alongside Laravel auto-discovery). `Analyze` also now folds `composer.DirectDevRequires` into the checked universe.

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
