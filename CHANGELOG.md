# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Undefined variable** diagnostic: warns on reads of unbound variables; skips superglobals, `$this`, parameters, foreach/catch/closure bindings, and scopes using `extract()`, `include`, or `eval`.
- **Unused variable** diagnostic: hints on local assignments whose value is never read; ignores underscore-prefixed names (`$_`, `$_foo`) and by-reference bindings.
- **Argument count mismatch** diagnostic: warns when a call passes too many arguments to a project function or static method with a fixed parameter list; skips variadic, spread, and named-argument calls.
- Per-rule severity in `.tusk-php.json`: `diagnosticRules` now accepts `"off"` / `"hint"` / `"info"` / `"warning"` / `"error"` in addition to the existing `true`/`false` toggle.
- `textDocument/selectionRange`: hierarchical selection expansion from word → brackets → method → class → document.
- `textDocument/semanticTokens/full`: semantic highlighting for keywords, strings, numbers, comments, variables, declarations, and property names.
- Match expression parsing: `ParseResult.Matches` captures every `match` structure in a file, including nested matches; recovers gracefully from malformed arms.
- Call-site model: `ParseResult.Calls` records every function, method, static, and constructor call with per-argument names, values, and spread flags (capped at 5 000/file).
- First-class callable detection: `strlen(...)` / `$obj->method(...)` syntax sets `CallSite.IsFirstClassCallable`, distinguishing it from regular and variadic calls.
- **Symfony translation intelligence**: completion and go-to-definition for translation keys across YAML, PHP array, and XLIFF files; understands `$translator->trans()`, `t()`, `new TranslatableMessage()`, and domain-argument forms.
- **Symfony `#[Autowire]` support**: constructor injection now resolves `service:`, `param:`, and `env:` attribute forms.
- **Symfony console command discovery**: indexes `#[AsCommand]` attributes and legacy `protected static $defaultName` declarations.
- `symfony_routes` and `symfony_route_to_controller` MCP tools, plus `tusk://symfony/routes` resource — Symfony workspaces only. Routes are indexed at workspace build time for instant responses.
- `php_graph` MCP tool: builds container, reference, or model dependency graphs in JSON, Mermaid, or DOT format (`kind`, `deps`, `format` inputs).
- `contractVersion: 1` field in `php_project_summary` so agents can detect breaking interface changes.
- Standalone `tusk-mcp` binary distributed alongside `tusk-php` for MCP tool hosting.
- `graph container`, `graph references`, and `graph models` CLI subcommands for exporting PHP dependency graphs.
- `deps tree`, `deps usage`, and `deps unused` CLI subcommands for visualising Composer dependency trees, measuring symbol usage, and finding unused packages.
- `--parse` flag: dumps a PHP file's parsed AST as JSON from the command line.
- `introspect` subcommand: dumps parsed state (symbols, diagnostics, active profile) for a file; also available in editors as `tuskPhpLsp.debugDocument`.
- `phpVersion` and `extensions` in `.tusk-php.json` now slot into the builtin profile chain between Composer constraints and local PHP detection.
- Four-step PHP version resolution: Composer platform → `.tusk-php.json` → local `php` binary → bundled default; the winning source is logged on startup.
- `db.allowDataSampling` and `db.connectTimeoutMs` configuration options for database introspection.
- PHP 8.1 stubs: `Fiber`, `array_is_list`, `enum_exists`, `UnitEnum`/`BackedEnum`, Reflection enums/fibers, `AllowDynamicProperties`, `ReturnTypeWillChange`, `CURLStringFile`.
- PHP 8.2 stubs: `ini_parse_quantity`, `SensitiveParameter`/`SensitiveParameterValue`.
- PHP 8.3 stubs: `Override` attribute.
- WASM (`wasip1`) build target for browser-based editors such as vscode.dev.
- `docs/mcp-tools.md`: reference for all 16 MCP tools and 6 resources, including schemas, output shapes, framework gating, and contract/schema versions.

### Changed

- Unified Composer `installedPackage` struct now includes `Version`; removed the `installedPackageWithVersion` duplicate.

### Fixed

- `workspace/executeCommand` capability now lists all three handled commands (`tuskPhpLsp.namespaceForPath`, `tuskPhpLsp.copyNamespace`, `tuskPhpLsp.moveToNamespace`); previously only the first was advertised.
- `laravel_*` MCP tools are now gated on Laravel framework detection and no longer appear in non-Laravel workspaces.
- Variadic parameters (`...$args`) are now correctly detected via the triple-dot token sequence.
- `ClassNode.EndLine` is now exposed in the compatibility layer.

## [0.8.0] - 2026-06-01

### Added

- Hover cards for `composer.json` `require` and `require-dev` entries: hovering a package name shows its installed version (read from `composer.lock`), version constraint, PHP requirement with a compatibility glyph against the workspace PHP version, license, description, and a link to the package's Packagist or VCS page.
- Go-to-Definition on a `composer.json` package name can open the package's Packagist (or configured repository) page in the browser, gated by the new `composer.openOnDefinition` setting and the editor's `window/showDocument` support.
- New settings `tuskPhpLsp.composer.hover.enable` (default on) and `tuskPhpLsp.composer.openOnDefinition` (default off) in the VS Code extension.
- VS Code extension now activates on workspaces containing a `composer.json` and routes JSON files matching that pattern to the server.
- Zed extension now registers JSON as a handled language so `composer.json` files are routed to the server for hover and definition.
- Attribute completion now surfaces indexed attribute classes (user-defined and from vendor) that match the typed prefix, with automatic `use`-import edits inserted alongside the existing built-in attribute suggestions.

### Fixed

- PHP 8 attributes with constructor arguments — such as `#[Char("name", nullable: true)]` — no longer generate spurious "unknown function" diagnostics for the call inside the attribute.
- Named arguments inside attribute constructors — such as `nullable: true` — no longer generate spurious "unknown attribute" diagnostics.

## [0.7.0] - 2026-05-28

### Added

- Generated PHP built-in symbol registry covering functions, classes, interfaces, and traits from PHP's internal runtime tables, preventing known PHP symbols from being reported as unknown when no richer stub exists yet.
- Embedded, versioned PHP stub layers for core symbols and extensions, including initial PHP 7.4/8.0 core stubs and JSON extension stubs through PHP 8.3.
- Project-aware builtin profile selection from Composer platform data, Composer `require.php` constraints, required `ext-*` packages, local PHP detection, and the bundled default profile.
- `builtin-unavailable` diagnostics for known PHP builtins that exist outside the project's selected PHP version or enabled extension set, such as PHP 8 functions used in a PHP 7.4 project.
- Local PHP version detection via the `php` binary, with configurable timeout handling and fallback to bundled stubs when detection is unavailable.
- PHP manual URL generation for builtin methods, properties, class constants, predefined constants, classes, interfaces, traits, enums, and functions.
- Configurable PHP manual locale through `php_manual_locale`.
- Optional browser opening for builtin definitions through `php_manual_open_on_definition`, using the LSP `window/showDocument` capability when supported by the client.
- VS Code settings for PHP manual locale and builtin definition opening.
- A builtin-generation helper script for registry, stub, and availability metadata generation.
- Additional resolver support for `class-string<T>` constructor binding, typed access-chain walking, `foreach` element propagation, and Reflection generic metadata.
- Unknown PHP attribute diagnostics, including detection for builtin attributes that are unavailable in the selected PHP profile.

### Changed

- Builtin registration now uses profile-aware stub layering, with generated fallback symbols loaded before richer curated stubs.
- LSP initialization now resolves and logs the effective PHP builtin profile from Composer, local PHP, or bundled defaults.
- Unknown-symbol diagnostics use a softer mode while indexing settles, reducing noisy false positives during startup.
- Hover and definition resolution now use declaring classes for inherited builtin members so PHP manual links point to the correct manual page.
- Hover resolution now prefers PHPDoc `@return` types over broad native placeholder types such as `object`, `mixed`, and `array` when that gives a more useful result.
- Documentation was refreshed for project architecture, builtin navigation, and the new PHP manual settings.
- Moved the Go module and repository references from `github.com/open-southeners/tusk-php` to `github.com/Tusk-PHP/lsp`, including internal Go imports, release/download URLs, and contributor documentation.

### Fixed

- Unknown-function and unknown-class diagnostics no longer flag broad PHP builtins such as `array_reverse`, `ReflectionObject`, `ReflectionMethod`, and `InvalidArgumentException`.
- Builtin symbols are filtered from workspace navigation results where jumping to generated or embedded builtin declarations would be unhelpful.
- Class properties are excluded from standalone name lookup, preventing property symbols from resolving as unrelated bare identifiers.
- Manual links for receiver-class and inherited-member hovers now strip leading namespace separators and resolve against the correct owner class.
- Hover and resolver behavior for unknown typed class variables, property access chains, and string-based variable assignments is more accurate.
- Attribute syntax and local PHP detection timeout handling are covered by the resolved diagnostic follow-ups.

## [0.6.1] - 2026-05-25

### Added

- Composer-generated `vendor/composer/autoload_files.php` parsing so global helper files registered through Composer are indexed even when package metadata is unavailable or incomplete.
- Additional PHP built-in function symbols for common array, regex, type-checking, constant, class, object, and function-introspection helpers.

### Fixed

- Hover cards no longer resolve words inside PHP string literals as unrelated symbols, preventing array declaration keys such as `'environment' => [...]` from showing constant or method hover cards.
- Unknown-function diagnostics no longer flag PHP built-ins such as `get_class()` or indexed Laravel/Composer global helpers such as `class_basename()`.

## [0.6.0] - 2026-05-22

### Added

- Configurable inlay hints for inferred variable types, `foreach` key/value types, closure and method return types, and call-site parameter names.
- New LSP navigation surfaces for type definitions, implementations, document highlights, folding ranges, and richer workspace symbol results.
- Native unknown-symbol diagnostics for unresolved classes, functions, and members.
- Code actions for unused imports, import organization, and unknown-class import quick fixes.
- Refactor code actions for implementing missing methods, generating constructors, generating getters and setters, extracting variables, and conservatively inlining local variables.
- Structured PHPDoc support for conditional type rendering and local type-alias resolution across completion, hover, and type resolution.
- Laravel route, view, and translation metadata indexes with completion and definition support for framework string references.
- Symfony route-name intelligence for attribute routes plus YAML/XML route config and imported route resources.
- Symfony container discovery for YAML and PHP service configuration, including fluent PHP `set()` and `alias()` definitions.

### Changed

- Renamed the server binary, bundled editor binaries, logs, and project config references from `php-lsp` to `tusk-php`.
- Cursor analysis and scoped variable tracking now model position-aware namespaces/imports, free functions, closures, arrow functions, grouped imports, closure captures, destructuring, and assignment metadata used by rename and refactor actions.
- Container service-ID navigation now distinguishes service-definition locations from type-definition targets.
- Inlay hints render shorter type names when fully qualified names can be reduced for display.

### Fixed

- Generic and docblock type resolution for annotated parameters and union generic carriers, including recursion isolation for concurrent resolver work.
- Parser recovery errors are preserved and published as syntax diagnostics with stable locations for malformed input.
- Enum interface implementations now participate in implementation lookup.
- Variable rename preparation no longer advertises unsupported unscoped variable renames.
- Position-aware cursor scope and import resolution prevent later namespace/import state from leaking into earlier source regions.

## [0.5.0] - 2026-05-17

### Added

- PHP 8.3–8.5 syntax support in the parser: property hooks (`{ get => ...; set => ...; }`), the `|>` pipe operator, asymmetric visibility (`public private(set)`), and dynamic class constant fetch (`Class::{$name}`) — constructs that were previously silently dropped or mis-parsed.
- Hover cards for properties now show PHP 8.4 property hooks and asymmetric `(set)` visibility.

### Changed

- Faster workspace-wide symbol prefix search via binary lookup, improving completion responsiveness on large projects.
- Find references now reuses indexed in-memory source instead of re-reading every file from disk on each request.
- Large files are indexed off the JSON-RPC message loop, keeping the server responsive while big documents are processed.

### Fixed

- Fatal crash (stack overflow) in hover and completion when resolving self-referential or mutually-referential variable assignments.
- Excessive work and apparent hangs in the variable-type chain resolver on certain repeated assignment patterns.
- Property hook bodies leaking their local variables (`$value`, `$this`) as spurious class properties in completion and document symbols.
- Completion returning blank entries when completing namespace segments in `use` statements.
- Completion results being returned in a non-deterministic order between identical requests.
- Signature help highlighting a parameter position past the end of the parameter list when more arguments than parameters were typed.

## [0.4.0] - 2026-03-25

### Added

- Laravel Facade support: static calls like `Cache::get()` now resolve through the container via `getFacadeAccessor()`, providing completions and hover cards from the concrete class's methods.
- Facade concrete method completion: methods not declared in `@method static` annotations but present on the resolved concrete class are surfaced as static completions.
- Nested trait member resolution at all depth levels (trait using another trait using another trait, etc.), propagating methods and properties through the full chain.

### Fixed

- `@method static` docblock annotations not setting `IsStatic` on virtual members, causing them to be filtered out of `ClassName::` completions.
- Trait `use` declarations inside other traits being silently discarded by the parser, preventing nested trait members from appearing in completions and hover.
- `traitMap` entries not being cleaned up on file re-index, causing stale trait associations to persist until process restart.
- Windows workspace path containing percent-encoded characters (`%3A` for `:`) not being decoded, causing the LSP server to index zero files.
- All `file://` URI-to-path conversions now use proper URL decoding and Windows drive letter handling via the shared `symbols.URIToPath` helper.

## [0.3.2] - 2026-03-24

### Fixed

- VSCode extension shipping without bundled LSP binaries due to release CI not copying platform binaries into the extension's `bin/` directory before packaging the `.vsix`.

## [0.3.1] - 2026-03-24

### Fixed

- VSCode extension failing to start the bundled LSP binary on Windows (`spawn tusk-php ENOENT`) by appending `.exe` to the PATH fallback command.
- VSCode extension failing to start the bundled LSP binary on Linux/macOS remote environments by ensuring execute permissions after `.vsix` extraction.
- Zed extension build failure caused by `unicode-segmentation` 1.13.0 breaking `heck` 0.4.1 (`UnicodeWords` made private); pinned to 1.12.0.

## [0.3.0] - 2026-03-24

### Added

- Generic-aware type resolution for template-annotated classes and methods, preserving concrete type parameters through completions, hover, and chained calls.
- Generic-aware support for Laravel Eloquent builders and collections, including concrete model propagation for methods such as `query()`, `where()`, `get()`, `first()`, `find()`, and related collection helpers.
- Array literal and array-shape inference in resolved expression types, improving type preservation for inferred variables and collection payloads.
- Local `scripts/set-version.sh` utility to update repository version strings for releases.

### Changed

- Release automation now targets plain semantic version tags without the previous release prefix.
- Expanded Laravel and Symfony end-to-end fixtures and coverage around generics, array shapes, and framework controller flows.

### Fixed

- Incorrect generic propagation across LSP completions, hover results, and shared resolver paths.
- Variable type inference for generic return values, including Laravel helpers such as `Arr::first()` and `Collection::first()`.
- Symfony test fixture compatibility issues that were causing CI instability.

## [0.2.0] - 2026-03-23

### Added

- Multi-source model attribute discovery for Eloquent and Doctrine ORM models.
- Virtual property injection from `@property`/`@method` docblock tags on classes.
- IDE helper file support (`_ide_helper_models.php`, `_ide_helper.php`) with member merging into existing indexed classes.
- Eloquent relation discovery from method return types and `$this->hasMany()`/`belongsTo()`/etc. calls.
- Eloquent accessor/mutator detection (both legacy `getNameAttribute()` and modern `Attribute` cast).
- Eloquent virtual static methods (`where`, `find`, `first`, `with`, `orderBy`, etc.) mimicking `__callStatic` forwarding.
- Doctrine ORM entity support: `#[Column]`, `#[Id]`, `#[GeneratedValue]`, `#[OneToMany]`, `#[ManyToOne]`, `#[OneToOne]`, `#[ManyToMany]` PHP 8 attribute parsing.
- Doctrine repository class detection and association to entities via `#[Entity(repositoryClass: ...)]`.
- Database schema introspection for MySQL, PostgreSQL, and SQLite via `.env` credentials with in-memory caching.
- Laravel migration file parsing to extract column definitions (`$table->string()`, `$table->foreignId()`, `$table->timestamps()`, etc.).
- Builder string argument completion for column names inside `where('`, `orderBy('`, `select('`, `pluck('`, `groupBy('`, etc.
- Builder string argument completion for relation names inside `with('`, `has('`, `whereHas('`, `load('`, `withCount('`, etc.
- Array argument support for Builder completions (`select(['`, `get(['`, `with(['`, etc.).
- Strict DB-only column filtering for `get()` (only suggests columns from IDE helper, database introspection, or migrations).
- `.env` file parser for reading database connection settings.
- `database` configuration option in LSP initialization settings to enable/disable DB introspection.
- Built-in diagnostics engine with standalone `internal/checks/` package reusable by CLI tools and CI pipelines.
- Unused import detection with word-boundary scanning, aliased imports, `use function`/`use const`, and PHP 8 attributes.
- Unused private method and property detection (excludes magic methods).
- Unreachable code detection after `return`, `throw`, `exit`/`die`, `continue`, `break`.
- Redundant union member detection (duplicates, `?Type|null`, supertype subsumption for `mixed`, `object`, `iterable`).
- Redundant nullsafe `?->` operator detection on non-nullable types.
- Unknown column validation in Builder string arguments (`where`, `orderBy`, `select`, `get`, etc.).
- Unknown relation validation in Builder string arguments (`with`, `has`, `whereHas`, `load`, etc.).
- Aggregate relation method second-arg validation (`withSum('relation', 'column')` checks column on the related model).
- `DiagnosticTag` support: unused code greyed out (`Unnecessary`), deprecated functions struck through (`Deprecated`).
- `diagnosticRules` configuration in `.tusk-php.json` to enable/disable individual diagnostic rules.
- Multi-line method chain resolution for hover, go-to-definition, and completions.
- `resolve.JoinChainLines()` helper to join continuation lines starting with `->`, `::`, or `?->`.
- Comprehensive chain resolution test suite covering single-line and multi-line Eloquent method chains.

### Fixed

- Hover cards and go-to-definition landing on wrong vendor symbols when method chains span multiple lines.
- Completion provider now resolves multi-line chains correctly instead of falling through to global completions.

## [0.1.0] - 2026-03-21

### Added

- Initial `Tusk PHP LSP` release with a Go-based PHP language server binary.
- VS Code extension packaging for the bundled LSP client.
- Zed extension packaging for the WebAssembly-based extension.
- Cross-platform release artifacts for Linux, macOS, and Windows.
