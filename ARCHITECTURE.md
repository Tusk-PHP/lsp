# Architecture

A Go-based Language Server Protocol (LSP) implementation for PHP, with deep Laravel and Symfony awareness. PHP 7.4 is the supported baseline; layered embedded stubs add 8.0+ symbols on demand based on the project's selected PHP profile. No external PHP parser — includes a custom tokenizer and lightweight AST.

## Project Structure

```
tusk-php/
├── cmd/tusk-php/main.go             # Entry point: CLI flags, stdio, server startup
├── internal/
│   ├── lsp/                         # JSON-RPC server, message dispatch, lifecycle
│   ├── protocol/                    # LSP type definitions (no logic)
│   │
│   ├── parser/                      # PHP tokenizer + lightweight AST (fault-tolerant)
│   ├── symbols/                     # Central symbol table (Index) and BuiltinProfile
│   ├── stubs/                       # Embedded PHP builtin stubs (per-version, per-extension)
│   │
│   ├── source/                      # Cursor-context abstraction (position, access kind, scope)
│   ├── scope/                       # Variable/binding scope collector
│   ├── resolve/                     # Symbol/type resolution (PHPDoc, generics, templates, aliases)
│   ├── types/                       # Type model primitives (named, union, intersection, generic)
│   ├── phparray/                    # PHP array literal helpers shared by parsers
│   ├── phpdetect/                   # Detects locally installed PHP version (timeout-bounded)
│   │
│   ├── hover/                       # Hover cards with type resolution + access chain walking
│   ├── completion/                  # Context-aware completions
│   ├── analyzer/                    # Definition, references, document/workspace symbols, rename, signature help
│   ├── inlayhint/                   # Inlay hints (variable types, parameter names, return types)
│   ├── checks/                      # Standalone PHP static analysis rules (no LSP dependency)
│   ├── diagnostics/                 # Wraps checks/ + PHPStan/Pint integration, publishes diagnostics
│   │
│   ├── composer/                    # composer.json autoload (PSR-4 + files) + platform detection
│   ├── container/                   # Laravel/Symfony DI container analysis
│   ├── config/                      # .tusk-php.json + client options + framework detection
│   ├── framework/laravel/           # Routes, views, translations
│   ├── framework/symfony/           # Routes and other Symfony-specific symbols
│   ├── models/                      # Eloquent, Doctrine, migrations, array shape inference
│   │
│   └── conformance/                 # Whole-stack invariants and corpus tests (build-tagged)
├── editors/
│   ├── vscode/                      # TypeScript extension
│   └── zed/                         # Rust/WASM extension
├── testdata/project/                # Test fixtures (mock PHP project)
├── scripts/                         # Build, corpus fetch, builtin generators
└── Makefile                         # Build, test, dev, conformance targets
```

## Entry Point

`cmd/tusk-php/main.go` parses CLI flags (`--version`, `--log`, `--stdio`, `--strict`), creates a `lsp.Server` with stdin/stdout, and calls `server.Run()` which enters the JSON-RPC message loop.

## JSON-RPC Protocol

Communication uses stdio with `Content-Length` headers. The server reads messages in a single-threaded loop, dispatches to handlers, and sends responses/notifications.

### Supported Methods

| Method | Purpose |
|--------|---------|
| `initialize` | Handshake: load config, detect framework, register builtins for resolved PHP profile |
| `initialized` | Async start: workspace indexing, composer deps, container analysis |
| `shutdown` / `exit` | Graceful termination (`exit` uses an injectable function for testability) |
| `textDocument/didOpen` | Store document, index, run diagnostics |
| `textDocument/didChange` | Update content, re-index, re-run diagnostics |
| `textDocument/didClose` | Drop in-memory document state |
| `textDocument/didSave` | Re-index, trigger PHPStan/Pint |
| `textDocument/completion` | Context-aware completions at cursor |
| `textDocument/hover` | Type info and docs at cursor |
| `textDocument/definition` | Jump to symbol definition |
| `textDocument/typeDefinition` | Jump to the declared type of a variable/property |
| `textDocument/implementation` | Jump to interface implementers (classes, enums) |
| `textDocument/references` | Find all references in the workspace |
| `textDocument/documentSymbol` | File outline (classes, methods, properties) |
| `textDocument/foldingRange` | Folding regions for blocks, PHPDoc, etc. |
| `textDocument/documentHighlight` | Highlight other occurrences of the symbol under cursor |
| `textDocument/signatureHelp` | Parameter hints in function calls |
| `textDocument/prepareRename` | Validate cursor target is renameable |
| `textDocument/rename` | Rename a symbol across the workspace |
| `textDocument/codeAction` | Quickfixes (remove unused import, organize imports, …) |
| `textDocument/inlayHint` | Inline type/parameter-name annotations |
| `workspace/symbol` | Workspace-wide symbol search |
| `workspace/executeCommand` | Server-side commands (reindex, …) |

## Core Data Structures

### Symbol

Represents any PHP symbol (class, method, property, function, constant, enum, etc.):

```go
type Symbol struct {
    Name          string
    FQN           string
    Kind          SymbolKind
    Source        SymbolSource             // Project, Builtin, or Vendor
    URI           string
    Range         protocol.Range
    Visibility    string
    SetVisibility string                   // PHP 8.4 asymmetric visibility
    IsStatic      bool
    IsAbstract    bool
    IsFinal       bool
    IsReadonly    bool
    Type          string                   // Property/constant type
    ReturnType    string                   // Method/function return type
    DocComment    string                   // PHPDoc block
    ParentFQN     string                   // Owning class FQN
    Params        []ParamInfo
    Children      []*Symbol                // Nested symbols (class → methods/properties)
    Implements    []string
    Extends       string
    BackedType    string                   // Enum backing type
    Value         string                   // Constant/enum-case value
    IsVirtual     bool                     // Synthesized from @property/@method docs (IDE helper)
    Templates     []TemplateParam          // @template T (of Bound)
    TypeAliases   map[string]TypeAlias     // @phpstan-type / @phpstan-import-type
    Hooks         []parser.PropertyHook    // PHP 8.4 property hooks (get/set)
}
```

### Index

Thread-safe central symbol table. All providers read from it:

```go
type Index struct {
    mu                   sync.RWMutex
    symbols              map[string]*Symbol   // FQN → Symbol
    nameIndex            map[string][]string  // Short name → [FQNs]
    fileSymbols          map[string][]*Symbol // URI → symbols in file
    namespaceIndex       map[string][]string  // Namespace → [FQNs]
    inheritanceMap       map[string]string    // Child → Parent FQN
    implementsMap        map[string][]string  // Class → [Interface FQNs]
    reverseImplementsMap map[string][]string  // Interface → [Implementer FQNs]
    traitMap             map[string][]string  // Class → [Trait FQNs]
    sortedNames          []string             // sorted lowercase names for binary search
    sortedFQNs           []string             // sorted FQN keys for binary search
    fileSource           map[string]string    // URI → raw source for in-memory lookups
    ready                atomic.Bool          // true once project + vendor indexing settled
}
```

Key methods: `IndexFileWithSource`, `IndexIDEHelperFile`, `Lookup(fqn)`, `LookupByName(name)`, `SearchByPrefix(prefix)`, `SearchByFQNPrefix(prefix)`, `GetClassMembers(classFQN)`, `GetInheritanceChain(classFQN)`, `RegisterBuiltinsForProfile(profile)`. The helper `symbols.IsBuiltin(sym)` is the canonical way to detect builtin symbols (URI prefix `builtin://` or empty URI with `SourceBuiltin`).

### BuiltinProfile

Selects which PHP builtin layers get indexed at startup:

```go
type BuiltinProfile struct {
    PHPVersion string   // e.g. "8.2"
    Extensions []string // e.g. ["json", "curl"]
}
```

Layer ordering (deterministic, later layers override earlier ones):

1. Generated fallback registry (cheap names-only entries from `phpstorm-stubs` generation)
2. PHP 7.4 core baseline (full stub file: `internal/stubs/php/core/php-7.4.php`)
3. Core version deltas up to `PHPVersion` (e.g. `php-8.0.php`, `php-8.1.php`, …)
4. Enabled extension baselines (`internal/stubs/php/extensions/<ext>/php-7.4.php`)
5. Extension version deltas up to `PHPVersion`
6. Hand-authored availability overrides in `symbols.builtinAvailabilityByName`

`stubs.BuiltinPHPForProfile(Profile)` returns the version/extension-filtered set of embedded stubs the index parses.

### FileNode

Result of parsing a PHP file (from `parser.ParseFile()`):

```go
type FileNode struct {
    Namespace  string
    Uses       []UseNode
    Classes    []ClassNode        // with Methods, Properties (Hooks, SetVisibility), Constants
    Interfaces []InterfaceNode
    Traits     []TraitNode
    Enums      []EnumNode         // with Cases, Methods
    Functions  []FunctionNode
    Errors     []ParseError       // recovered syntax errors, surfaced as `syntax-error` diagnostics
}
```

## Initialization Sequence

```
Client                              Server
  │                                   │
  │──── initialize ──────────────────>│  Load config, detect framework,
  │                                   │  resolve BuiltinProfile (composer
  │                                   │  platform → .tusk-php.json → phpdetect),
  │                                   │  index.RegisterBuiltinsForProfile(profile),
  │<─── InitializeResult ────────────│  advertise server capabilities
  │                                   │
  │──── initialized ─────────────────>│  Spawn concurrent goroutines:
  │                                   │    1. indexWorkspace()
  │                                   │    2. indexComposerDependencies()
  │                                   │    3. container.Analyze()
  │                                   │  Once all settle, re-publish diagnostics
  │                                   │  for open documents with unknown-symbol
  │                                   │  findings re-enabled
```

**indexWorkspace**: Walks filesystem, indexes `.php` files as `SourceProject`, skips excluded dirs.

**indexComposerDependencies**: Parses `composer.json` and `vendor/composer/installed.json` for PSR-4 directories and `autoload.files`. Indexes vendor PHP files as `SourceVendor`. Supports Composer v1 and v2.

**container.Analyze**: Laravel → scans service providers for `bind`/`singleton` and pre-loads 25+ framework bindings. Symfony → parses `services.yaml`, PHP config files, and `#[Autowire]` attributes.

**Soft-mode diagnostics**: Unknown-symbol findings are suppressed on the first publish (the index isn't yet fully populated, so they'd produce false positives). Once `indexWg` settles, diagnostics are re-published for open documents with the full rule set enabled.

## How Indexing Works

`IndexFileWithSource(uri, source, src)`:

1. Parse source → `FileNode` (namespace, uses, classes, functions, etc.)
2. Lock index, remove old symbols for this URI
3. For each declaration: build FQN, resolve type names via `use` imports, create `Symbol` with source tag
4. Store in all index maps (FQN, name, file, namespace, inheritance/implements/trait)
5. Cache raw source for in-memory occurrence scans (used by find-references)

**Type resolution**: Short names → FQN via `use` statements → current namespace fallback → global.

**Symbol sources**: `SourceProject` (workspace files), `SourceBuiltin` (PHP built-ins, URI prefix `builtin://`), `SourceVendor` (composer dependencies). Used for completion sorting and navigation gating.

## Package Responsibilities

### Foundational layers (consumed by every provider)

#### parser

Custom PHP tokenizer + structural parser. Produces `FileNode` with classes, methods, properties, functions. Handles PHP 8.0–8.5 syntax: union/intersection types, enums, readonly classes, attributes (`#[Name]`), named arguments, **property hooks** (`{ get => ...; set => ...; }`), **asymmetric visibility** (`public private(set)`), **pipe operator** (`|>`), **dynamic class constant fetch** (`Class::{$name}`). Fault-tolerant: bail-out conditions, progress guards, partial results on panic, recovered syntax errors recorded on `FileNode.Errors`.

#### symbols

The `Index` is the shared data store. All providers receive `*Index` and query it. Handles FQN resolution, inheritance chain traversal, trait merging, namespace membership. `PickBestStandalone()` selects the most appropriate symbol when multiple share a name (prefers functions over methods in standalone context). `BuiltinProfile` and `RegisterBuiltinsForProfile` drive layered builtin indexing.

#### stubs

Embedded PHP builtin stub files under `internal/stubs/php/core/` (language) and `internal/stubs/php/extensions/<ext>/` (extension symbols), named `php-<version>.php`. PHP 7.4 files are full baselines; later version files are deltas. `BuiltinPHPForProfile(Profile)` returns the deterministic, version/extension-filtered set of stubs the index parses.

#### source

Shared cursor-context abstraction. `sourcectx.Analyze(uri, source, pos)` classifies the cursor (`AccessKind`, `ContextSymbolKind`, enclosing `Scope`, active namespace, active imports) so providers don't re-derive cursor state. Models named functions, anonymous closures, arrow functions, methods, and tracks namespace/import state at the cursor position rather than file-globally.

#### scope

Reusable scope/variable collector for parameters, closures (including implicit `fn (...) =>` captures), `foreach`/`catch` bindings, `list(...)` destructuring, and grouped imports. Required for undefined/unused variable diagnostics and scoped rename.

#### resolve

Centralized type/symbol resolution. Hosts PHPDoc parsing, generic templates, type aliases (`@phpstan-type` / `@phpstan-import-type`), class-string bindings, member-chain walking, and re-entrancy depth-guarded variable type resolution. Completion, hover, and diagnostics go through this layer so they agree on what type a variable holds.

#### types

Type model primitives (`TypeKind`: named, union, intersection, generic, …) used by `resolve/`.

#### phparray

Shared utilities for parsing PHP array literals — used by config parsing, container analysis, and completion.

#### phpdetect

Runs the local PHP binary to read its version, bounded by an explicit timeout (default 1 s, overridable via `config.PHPDetectTimeoutMs`). Exports `ErrTimeout` so callers can distinguish "PHP took too long" from "binary missing".

### LSP providers

#### hover

Resolves the symbol under the cursor and renders a markdown card:

1. Bold FQN header
2. Docblock summary
3. PHP code block with declaration (renders asymmetric visibility and property hooks)
4. Context (parent class, override/implements info, container bindings)
5. PHPDoc details (params, returns, throws, tags)
6. PHP manual link (for builtins)

Access chain resolution walks `$this->logger->info()` by resolving types at each step through the index. Primitive types (`string`, `int`, etc.) produce no hover. Variables typed with primitives show `type $varName` in the code block.

#### completion

Detects cursor context (via `source/`) from the line prefix:

| Context | Detection | Result |
|---------|-----------|--------|
| `$obj->` / `$obj?->` | Trailing `->` or `?->` | Instance methods, properties |
| `Class::` | Trailing `::` | Static methods, constants, enum cases |
| `new ` | Last word is `new` | Class names |
| `use Ns\` | Starts with `use`, contains `\` | Namespace segments + symbols |
| `\Ns\` | Contains `\` | Namespace segments + symbols |
| `#[` | Contains `#[` | Attribute names |
| `app(` / `make(` | Container resolve call | Container-bound class names |
| (default) | None of above | Types, functions, keywords, `$this` |

**Sort order**: `"0"` types → `"1"` same-namespace project → `"2"` other project → `"3"` builtins → `"4"` vendor → `"5"` keywords. All return paths funnel through a sanitizer that drops empty-`Label` items and produces deterministic ordering.

Variable type resolution (delegated to `resolve/`): method parameters → `$var = new Class()` assignments → `$var = 'literal'` inference → `@var` annotations → class properties.

#### analyzer

- **FindDefinition / FindTypeDefinition**: Resolves symbol → returns `Location`. Returns nil for unresolved-receiver access chains rather than matching unrelated same-name symbols (W1 guard).
- **FindImplementation**: Looks up `reverseImplementsMap` to find classes/enums implementing an interface.
- **FindAllReferences**: Uses the index's in-memory cached source (no per-call disk re-read). Skips built-in symbols — they're opaque to navigation.
- **GetDocumentSymbols** / **GetWorkspaceSymbols**: File-level and workspace-level outlines. Workspace filter excludes builtins via `symbols.IsBuiltin`.
- **GetSignatureHelp**: Identifies enclosing function call and clamps `activeParameter` to valid range.
- **PrepareRename / Rename**: Scoped through `scope.Collect`; rejects builtins (via `symbols.IsBuiltin`) and unresolved access chains.

#### inlayhint

Five categories of inline annotations: variable types, foreach variable types, closure return types, method return types, and call-site parameter name labels.

#### checks

Standalone PHP static analysis rules with **no LSP dependency** — results are `Finding` values usable from CI, CLI, or LSP. Current rules: `unknown-class` / `unknown-function` / `unknown-member`, `unused-imports`, `unused-private`, `unreachable`, `redundant-types`, `invalid-builder-args`, `builtin-unavailable` (functions, class-likes, attributes, constants).

#### diagnostics

Two layers:

1. **Fast checks** (on every change, debounced): runs the `checks/` rule set plus parser recovery errors as `syntax-error`. Results published via `publishDiagnostics`.
2. **External tools** (on save): PHPStan (JSON output → diagnostics with token-level ranges) and Laravel Pint (diff output → style warnings). Results cached per-file.

PHPStan diagnostic ranges highlight the specific identifier mentioned in the error message, falling back to the trimmed line content.

### Project context

#### composer

Parses `composer.json` (PSR-4 + `autoload.files`) and `vendor/composer/installed.json`. Returns `[]AutoloadEntry` with path, namespace, vendor flag, and file flag. `GetPlatform()` extracts the effective PHP version + `ext-*` set from `config.platform` / `require` for `BuiltinProfile` selection. Supports Composer v1 and v2.

#### container

Analyzes DI container bindings. **Laravel**: scans service providers for `bind`/`singleton` calls, pre-loads 25+ framework bindings. **Symfony**: parses `services.yaml`, PHP config files, and `#[Autowire]` attributes. Used by completion (container resolve context) and hover (binding info).

#### config

Loads `.tusk-php.json` and merges with LSP client `initializationOptions`. Auto-detects framework from `artisan` (Laravel), `bin/console` (Symfony), or `composer.json` requires. Hosts diagnostic severities, per-check enable/disable flags, and `PHPDetectTimeoutMs`. Defaults: 10000 max index files, excludes `vendor`/`node_modules`/`.git`/`storage`.

#### framework/laravel

Routes, views, translations — Laravel-specific symbol resolution that doesn't belong in generic completion.

#### framework/symfony

Routes and other Symfony-specific symbol resolution.

#### models

Eloquent (relations/casts/scopes/accessors), Doctrine, migrations, array shape inference. Sources synthetic `IsVirtual` symbols (from `@property`/`@method` IDE helper files) into the index.

### Server and conformance

#### lsp

The `Server` struct owns all providers, the index, configuration, and the document store. `Run()` is the JSON-RPC message loop. `handleMessage()` dispatches by method. Documents stored in `sync.Map`. Panic recovery wraps all handlers and goroutines via `recoverPanic` / `goSafe`. Strict mode (`--strict` / `TUSK_STRICT`) re-panics recovered errors for fatal termination, including from background goroutines. `exit` calls an injectable `exitFunc` so the full lifecycle is testable.

#### conformance

Build-tagged (`//go:build conformance`) invariants and corpus tests that exercise the whole parser/index/provider stack. The real-world PHP corpus is not vendored — fetch it with `scripts/fetch-corpus.sh` before running `make conformance`.

## Editor Extensions

### VSCode (`editors/vscode/`)

TypeScript. Discovers binary from settings → bundled `bin/{platform}-{arch}/tusk-php` → PATH. Passes user settings as `initializationOptions`. Watches `.php` and `composer.json` files. Commands: restart, reindex.

### Zed (`editors/zed/`)

Rust/WASM. Zero user configuration. Downloads binary from GitHub releases for current platform. Falls back to PATH. Server must have sensible defaults since Zed users cannot configure options.

## Concurrency Model

- **Index**: `sync.RWMutex` — multiple concurrent readers, exclusive writer. `sortedNames` / `sortedFQNs` rebuilt under the write lock when their dirty flags are set (powers binary-search prefix lookups).
- **Documents**: `sync.Map` — lock-free concurrent access.
- **Message loop**: Single-threaded in `Server.Run()`.
- **Background work**: `goSafe()` spawns goroutines with panic recovery. Large documents (>5000 lines) are indexed asynchronously off the message loop; smaller ones stay synchronous for deterministic test behavior.
- **Providers**: Stateless query objects, safe for concurrent use.

## Request Flow Example

```
User hovers over "$this->logger->info()" on "info"
  ↓
VSCode sends textDocument/hover {uri, position}
  ↓
Server.handleHover()
  ↓
hover.Provider.GetHover(uri, source, position)
  ├─ sourcectx.Analyze() → AccessKind = instance, subject = "$this->logger"
  ├─ resolveAccessChain() walks left:
  │   "info" ← "->" ← "logger" ← "->" ← "$this"
  │   $this → App\Service (enclosing class from sourcectx)
  │   logger → property type Monolog\Logger (via resolve/)
  │   returns "Monolog\Logger"
  ├─ findMember("Monolog\Logger", "info") → Symbol
  └─ formatHover(symbol) → markdown card
  ↓
Server.sendResponse(id, hover)
  ↓
VSCode renders hover popup
```
