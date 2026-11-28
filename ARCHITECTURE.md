# Architecture

Tusk PHP is a PHP language server written in Go, with deep Laravel and Symfony awareness. It has its own fault-tolerant tokenizer and lightweight AST, so no PHP runtime is needed to analyse code. PHP 7.4 is the baseline for builtin symbols; newer versions (up to 8.6, the default when a project doesn't say) and extensions are layered on from the project's `composer.json`.

This document is the map: what each part does and how a request flows through it. The rules for changing the code (package layers, performance and correctness rules, the target structure) are in [AGENTS.md](AGENTS.md). User-facing behaviour is documented in [docs/](docs/).

## One binary, several front ends

`cmd/tusk-php` builds a single `tusk-php` binary:

| Command | What it does | Docs |
|---|---|---|
| `tusk-php` (`--stdio`) | Language Server Protocol over stdio, the default | [docs/editors.md](docs/editors.md) |
| `tusk-php mcp` | MCP server over stdio for AI agents (`mcp dump` writes an AI context pack to a folder) | [docs/ai-agents.md](docs/ai-agents.md) |
| `tusk-php check` | Reports the diagnostics the server would show, as text or JSON, with baselines | [docs/command-line.md](docs/command-line.md) |
| `tusk-php introspect` | Dumps what the server parsed and resolved for one file | |
| `tusk-php deps` | Composer dependency usage (`deps unused` lists packages with no static reference) | |
| `tusk-php -parse <file>` | Prints a file's AST as JSON | |

Other flags: `--version`, `--log <file>`, `--strict` (re-panic after recovering, also `TUSK_STRICT`).

The same code builds for WebAssembly (`GOOS=wasip1`, `make wasm`), so the server can run as a web worker (for example on vscode.dev). Platform differences live in build-tagged files (`*_wasip1.go`, `*_wasm.go`), such as stdin handling, memory limits and the live database connection.

Every front end builds its analysis state the same way, through `workspace` (see [Startup](#startup)), and gets its providers from `session`, which wires diagnostics, completion, hover, inlay hints and the analyzer over it once. The LSP adds only what is its own: client commands, lazy completion resolve, the Composer hover card and the coverage store.

## Packages

Packages are layered: a package imports only from its own layer or the ones above it. [AGENTS.md](AGENTS.md#architecture-today) holds the rule and the known exception.

```
cmd/tusk-php/                 Entry point: flags, subcommands, stdio
internal/
├── protocol/                 1. LSP wire types, no logic
│
├── document/                 2. Columns between byte offsets and the client's position encoding
├── parser/                      Tokenizer and lightweight AST (fault-tolerant)
├── types/                       Type model primitives (named, union, intersection, generic, …)
├── phparray/                    PHP array literal helpers
├── stubs/                       Embedded builtin stubs, per PHP version and extension
├── phpdetect/                   Reads the local PHP version (timeout-bounded)
│
├── symbols/                  3. The central index and builtin profiles
├── scope/                       Variables, closures, imports
├── resolve/                     Types, PHPDoc, generics, member chains
├── source/                      What is under the cursor
│
├── config/                   4. .tusk-php.json, client options, framework detection
├── composer/                    composer.json, installed.json, platform; cardhover/, lockfile/, manifest/
├── coverage/                    Coverage reports from PHPUnit and Pest
├── container/                   Laravel and Symfony service container bindings
├── models/                      Eloquent, Doctrine, migrations, database schema, array shapes
├── framework/laravel/           Routes, views and view data, translations, components, registrations
│   └── blade/                   Blade templates as virtual PHP
├── framework/symfony/           Routes, translations, console commands
├── framework/magento/           Classes Magento generates at build time
├── workspace/                   Builds the index for a root: files, Composer dependencies, framework analysis
│
├── completion/ hover/        5. Features
├── analyzer/                    Definition, references, symbols, rename, signature help
├── inlayhint/
├── checks/                      Native rules, no LSP types
├── diagnostics/                 Runs checks, plus PHPStan and Pint
├── symquery/ introspect/ deps/ unuseddeps/ graph/
├── session/                     Wires the providers above once, for the LSP, MCP and CLI
│
├── lsp/                      6. LSP transport
├── mcpserver/                   MCP transport
│
└── conformance/              7. Whole-stack invariants and corpus tests (build-tagged)
```

## Core data

These are the shapes everything else queries. The source is authoritative; only the key fields are listed here.

### Symbol (`symbols/index.go`)

Any PHP declaration: class-like, method, property, function, constant, enum case. It carries its `FQN`, `Kind`, `Source` (project, vendor or builtin), location, modifiers, declared and PHPDoc types (`Type`, `ReturnType`, `Params`, `DocComment`), and its children (a class's members). It also records:

- inheritance (`Extends`, `Implements`) and `Supertypes` with their generic type arguments;
- `Templates` and `TypeAliases` (`@template`, `@phpstan-type`);
- PHP 8.4 `Hooks` and `SetVisibility`, and `Attributes`;
- `IsVirtual` for members synthesised from `@property` / `@method` or IDE helper files, and `IsConditional` for a class declared inside a block (a placeholder under `if (false)`, a polyfill under `if (!class_exists(...))`), whose members may not be the ones code runs with.

`symbols.IsBuiltin(sym)` is the one way to tell a builtin symbol.

### Index (`symbols/index.go`)

The thread-safe symbol table every provider reads. Besides FQN, short-name and per-file maps it keeps:

- **lookup indexes:** a case-insensitive FQN index (PHP matches class and function names case-insensitively) and sorted name/FQN lists for prefix search;
- **hierarchy, both directions:** child → parent and parent → children, class → interfaces and interface → implementers, interface → sub-interfaces, class → traits;
- **member tables** by class and name, and a **generation counter** that caches derived from the index (resolution, diagnostics) check to know when they are stale;
- **framework hooks:** call forwards (a class that forwards unknown calls, like an Eloquent model to its query builder), scope conventions for builders, and a class generator that builds Magento's generated classes on a lookup miss and drops them when their source class changes;
- the raw source of indexed files, used by find-references.

Indexing a file parses it outside the lock, then replaces that file's entries in one short critical section.

### Builtin profile (`symbols/`, `stubs/`)

A `BuiltinProfile` is a PHP version and a set of extensions. Builtins are registered in a fixed order, later layers overriding earlier ones:

1. the generated registry: every builtin function's native signature, from reflection of PHP's builds, with the PHPDoc types PHPStan's function map gives it for the selected version where they say more than the native ones (`builtin_signatures_generated.go`, `scripts/generate-builtins.php signatures`); the built-in classes' members carry theirs the same way (`builtin_classes_generated.go`, `classes` mode); then the remaining names;
2. the PHP 7.4 core baseline (`stubs/php/core/php-7.4.php`) and the per-area core stubs (`spl/`, `date/`, `reflection/`, `io-poll/`, `time/`, `uri-*/`, …);
3. core version deltas up to the selected version (`php-8.0.php` … `php-8.6.php`);
4. extension baselines (`stubs/php/extensions/<ext>/php-7.4.php`);
5. extension version deltas;
6. hand-written availability entries (`builtinAvailabilityByName`), which say since which version a builtin exists, for `builtin-unavailable`.

### FileNode (`parser/`)

What `parser.ParseFile()` returns: the namespace and `use` imports, classes (including anonymous ones), interfaces, traits, enums, functions, top-level constants, and the syntax errors it recovered from (reported as `syntax-error`). The parser handles syntax up to PHP 8.6, including property hooks, asymmetric visibility, the pipe operator, partial function application and parameter doc comments. It is fault-tolerant: bail-out conditions, progress guards and partial results on panic.

## Startup

```
Client                              Server
  │──── initialize ─────────────────>│  Load config, detect framework, pick the
  │                                  │  builtin profile (composer platform →
  │                                  │  .tusk-php.json → local PHP), register
  │<─── InitializeResult ───────────│  builtins, create the workspace
  │                                  │
  │──── initialized ────────────────>│  In the background (goSafe):
  │                                  │   - index workspace files
  │                                  │   - index Composer dependencies
  │                                  │   - container analysis
  │                                  │   - coverage reports (read, then watched)
  │                                  │   - Laravel routes
  │                                  │   once both indexes settle:
  │                                  │   - analyse models, Symfony commands
  │                                  │   - re-publish diagnostics for open files
```

`workspace` owns these steps (`IndexWorkspace`, `IndexComposerDependencies`, `ScanRoutes`, `AnalyzeModels`, `ScanSymfonyCommands`). The LSP server runs them in the background so it can answer straight away; `tusk-php check`, `introspect` and the MCP server call `workspace.Build`, which runs them before returning.

Until indexing settles, unknown-symbol findings are held back, since the index isn't complete yet and they would be false. Once it settles, diagnostics for open files are published again with every rule on.

Composer: project files are indexed as `SourceProject`; the PSR-0, PSR-4, classmap and `files` autoload entries of installed packages (Composer 1 and 2) as `SourceVendor`.

## The LSP server

### Message loop and output

`Server.Run()` reads one JSON-RPC message at a time and dispatches it (`dispatch.go`). Each handler runs under `recoverPanic`; background work runs through `goSafe`, which recovers panics too. In strict mode both re-panic.

Requests are handled on the loop, so the loop does no work that grows with the workspace. Indexing, workspace-wide work, PHPStan and Pint run in the background. All output goes through one writer (`output.go`), so frames from the loop and from background goroutines never interleave.

### Documents

Positions inside the server count columns in bytes. `initialize` picks the client's column unit: UTF-8 when the client offers it in `general.positionEncodings`, otherwise UTF-16, the LSP default, and advertises it as `positionEncoding`. Every handler converts request positions to bytes and every position it sends back to the client's unit, in `lsp/positions.go`, against the text of the file each position belongs to (the open document, else the source the index keeps, else the file on disk, read at most once per request). `document.Columns` does the conversion; a line that is ASCII up to the column needs none.

Open documents are a map guarded by an `RWMutex`. On `didChange` the server stores the new text and re-indexes the file: synchronously for files up to 5,000 lines, otherwise through a background queue that only ever indexes the latest version. Diagnostics run in the background after a 200 ms pause in typing, one pending run per file; a result for text that has changed since is dropped.

### Methods

| Method | Handled by |
|---|---|
| `initialize`, `initialized`, `shutdown`, `exit` | `lsp` |
| `textDocument/didOpen`, `didChange`, `didClose`, `didSave` | `lsp` (documents, indexing, diagnostics) |
| `textDocument/completion`, `completionItem/resolve` | `completion` |
| `textDocument/hover` | `hover` (`composer/cardhover` for `composer.json`) |
| `textDocument/definition`, `declaration`, `typeDefinition`, `implementation`, `references` | `analyzer` |
| `textDocument/prepareTypeHierarchy`, `typeHierarchy/supertypes`, `typeHierarchy/subtypes` | `analyzer` |
| `textDocument/documentSymbol`, `workspace/symbol` | `analyzer` |
| `textDocument/documentHighlight`, `foldingRange` | `analyzer` |
| `textDocument/documentLink` | `analyzer` (`framework/laravel` for the path helpers) |
| `textDocument/selectionRange` | `lsp` |
| `textDocument/signatureHelp` | `analyzer` |
| `textDocument/prepareRename`, `rename` | `analyzer` |
| `textDocument/codeAction` | `analyzer`: import fixes, unknown-class fixes, implement missing methods, constructor and getter/setter generation, extract and inline variable |
| `textDocument/inlayHint` | `inlayhint` |
| `textDocument/semanticTokens/full` | `lsp` |
| `workspace/executeCommand` | `lsp`: `tuskPhpLsp.namespaceForPath`, `copyNamespace`, `moveToNamespace`, `createObject`, `copyReference`, `debugDocument` |
| `tusk/coverageReport`, `tusk/fileCoverage` | `coverage` |

The last row is behind the experimental `tuskCoverage` capability, for editors that show coverage.

Blade views (`.blade.php`) are compiled to virtual PHP (`framework/laravel/blade`) with a map back to the template. Hover, completion, definition, references and diagnostics run on that PHP and are mapped back; Blade's own directives, components and view names are answered by `lsp/blade_features.go`. Blade files are not added to the symbol index.

## The MCP server

`mcpserver` serves tools for AI agents over stdio: symbol lookup and explanation, references, diagnostics, project summary, container bindings, coverage, Laravel and Symfony routes, Laravel model schemas, and the database schema (`db_*`). It builds the workspace with `workspace.Build` and wires its own providers. See [docs/ai-agents.md](docs/ai-agents.md).

## Features

### Shared foundations

Every feature goes through these, so completion, hover, definition, references, rename, inlay hints and diagnostics agree:

- **`source`** (package `sourcectx`, `sourcectx.Analyze`): what is under the cursor: access kind (instance, static, …), the enclosing function, closure, method and class, and the namespace and imports in effect at that position.
- **`scope`**: variables, parameters, closure captures (including implicit `fn` captures), `foreach`/`catch` bindings, destructuring, grouped imports.
- **`resolve`**: types. PHPDoc (unions, intersections, shapes, literals, conditional types, `@phpstan-`/`@psalm-` tags), generics and templates, type aliases, class-string bindings, variable types with branch-aware narrowing, and one member-chain walker, `Resolver.AccessOwnerType`, that every feature uses. Results are memoised per index generation.

### Providers

- **`completion`**: classifies the cursor through `source` (member access, static access, `new`, namespaces and `use`, attributes, container calls, framework strings such as route, view, config and translation names) and ranks results: types, then same-namespace project symbols, other project symbols, builtins, vendor symbols, keywords.
- **`hover`**: a markdown card: the declaration as PHP (with asymmetric visibility and property hooks), its PHPDoc, where it comes from (parent, override, container binding), coverage, and a PHP manual link for builtins.
- **`analyzer`**: definition, type definition, implementation (through the reverse hierarchy maps), references (from the index's cached source, never re-reading disk), document and workspace symbols, signature help, rename. Builtins are never renamed or navigated into, and an unresolved receiver gives no answer rather than a guess.
- **`inlayhint`**: variable types, `foreach` variable types, closure return types, method return types, and parameter names at call sites.

### Diagnostics

Three sources, kept apart:

1. **Native rules** (`checks`, run by `diagnostics`): unknown classes, functions and members, undefined and unused variables, unused imports and private members, argument counts, deprecations, unavailable builtins for the project's PHP version, invalid overrides, unreachable code, redundant types and nullsafe operators, unknown Eloquent columns and relations, and more. `checks` has no LSP types, so the same rules serve `tusk-php check` and MCP. Every rule, its default severity and how to change it are in [docs/diagnostics.md](docs/diagnostics.md).
   - Cheap rules run after the 200 ms pause on the edited file only.
   - Heavier, type-aware rules run on save (`AnalyzeOnSave`); `tusk-php check` and MCP run both tiers.
2. **PHPStan**, on save, when the project has it: its JSON output, with ranges narrowed to the identifier each message names.
3. **Laravel Pint**, on save, when the project has it: style differences as warnings.

### The engine boundary

The packages the editor doesn't need to analyse code (everything under `internal/` except the transports, the editor features, `session` and the CLI-only tools) are the analysis engine. They never import `protocol`, the transports or the editor features: they use `textpos` positions, read client settings as `config.ClientOptions` and return `diagnostics.Diagnostic`, and `session.LSPDiagnostics` gives those the LSP's shape at the edge. `TestEngineBoundary` in `conformance` fails when an engine package imports across.

The engine stays in this module. Moving it into a module or repository of its own was planned and dropped ([#527](https://github.com/Tusk-PHP/lsp/issues/527)): it would have to publish nearly the whole core as API that [#140](https://github.com/Tusk-PHP/lsp/issues/140) is about to change, and turn most fixes, which touch the core and an editor feature together, into two changes and a version bump.

## Project context

- **`config`** merges `.tusk-php.json` with the client's `initializationOptions` and detects the framework (`artisan`, `bin/console`, `bin/magento`, or `composer.json`). Every option is in [docs/configuration.md](docs/configuration.md).
- **`composer`** reads `composer.json`, `composer.lock` and `vendor/composer/installed.json`: autoload entries, the PHP version and extensions for the builtin profile, suggested packages, and Laravel/Symfony package discovery. `cardhover`, `lockfile` and `manifest` power hover and definition inside `composer.json`.
- **`container`**: Laravel service providers (`bind`, `singleton`, …) and framework bindings; Symfony `services.yaml`, PHP config and `#[Autowire]`.
- **`models`**: Eloquent attributes, casts, relations, scopes and accessors; Doctrine entities; table columns from migrations (read from tokens) or, when configured, the live database; IDE helper files.
- **`framework/laravel`**: routes, views and the data each view receives, translations, Blade components and provider registrations (directives, components, view namespaces), and call forwarding for facades and models.
- **`framework/symfony`**: routes, translations and console commands.
- **`framework/magento`**: generated classes (`*Factory`, `*\Proxy`, `*\Interceptor`, `*SearchResults`, `*ExtensionInterface`, `*Extension`), built from their source class on demand.
- **`coverage`**: reads coverage reports from PHPUnit and Pest (it never runs tests) for hover, the editor's coverage view and MCP.

## Concurrency

- **Index**: `sync.RWMutex`. The lock is held only to read or swap data; files are parsed and their symbols built outside it.
- **Documents**: a map under an `RWMutex`; large-file indexing goes through a per-document queue that keeps only the latest version.
- **Message loop**: single-threaded, one message at a time.
- **Background work**: `goSafe` goroutines with panic recovery; their results are checked against the document version before they are used.
- **Output**: one writer, so frames never interleave.
- **Providers**: stateless query objects over the index.

Concurrent, cancellable request handling and versioned document snapshots are planned ([#114](https://github.com/Tusk-PHP/lsp/issues/114)).

## Request flow example

```
User hovers "info" in $this->logger->info()
  ↓
textDocument/hover {uri, position}
  ↓
Server.handleHover → hover.Provider.GetHover(uri, source, position)
  ├─ sourcectx.Analyze → instance access, subject "$this->logger"
  ├─ resolve.Resolver.AccessOwnerType walks left (the walker completion,
  │  definition, inlay hints and diagnostics share):
  │    "info" ← "->" ← "logger" ← "->" ← "$this"
  │    $this  → App\Service (enclosing class)
  │    logger → property type Monolog\Logger
  ├─ member lookup: Monolog\Logger::info
  └─ format the hover card
  ↓
response written through the single output writer
```

## Tests

- Unit tests sit next to the code and use the mock project in `testdata/project/` (`make test`, with `-race`).
- `conformance` (build tag `conformance`) checks whole-stack invariants over a real-world PHP corpus fetched by `scripts/fetch-corpus.sh` (`make conformance-pr`, `make conformance`).
- Field baselines hold the diagnostics for Laravel, Symfony, WordPress and Magento 2 at pinned commits in `testdata/field/*.baseline`; CI fails when findings appear or vanish (`make field-baseline`).
- `bench/` compares speed, memory and correct answers with other PHP language servers (`make bench`).

## Editor extensions

The extensions live in their own repositories and download a released `tusk-php` binary, pinned by version and checked against the SHA-256 sums published with the release:

- **VS Code**: [`Tusk-PHP/vscode`](https://github.com/Tusk-PHP/vscode), which passes its settings as `initializationOptions`.
- **Zed**: [`Tusk-PHP/zed`](https://github.com/Tusk-PHP/zed). Zed users can't configure the server, so every default has to work without configuration.
