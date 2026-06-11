# Roadmap

This is the technical roadmap for Tusk PHP, a high-performance PHP language
server written in Go with framework-aware Laravel and Symfony intelligence.

It replaces the original roadmap (formerly kept under `.claude/plans/`), most
of which has been delivered. This revision records what shipped, what the
current architecture can and cannot do, and the prioritized direction from
here (as of June 2026, around v0.9.0).

## Where The Project Stands

The original plan was: build shared foundations first, then protocol surface,
then diagnostics, then quickfixes, then framework depth. That sequence was
executed. The project now also has surfaces the original roadmap only
sketched as "longer term": an MCP server for AI agents, a graph-export CLI,
and a WASM (vscode.dev) build target.

### Delivered: foundations

- **`source/`** — shared cursor context (position classification, access kind,
  subject expression, enclosing scope, position-aware namespace/imports,
  arrow-function scopes).
- **`scope/`** — variable scope collector (parameters, closures with captures,
  foreach/catch bindings, destructuring including `list(...)`, grouped
  imports), with occurrence tracking used by rename.
- **`resolve/` + `types/`** — centralized type/symbol resolution over a
  structured type model: unions, intersections, nullable, generics
  (`Collection<int, User>`), array shapes, callables, literal types,
  `class-string<T>` constructor binding, `@template` substitution,
  `@phpstan-type` / `@phpstan-import-type` aliases with cycle detection, and a
  re-entrancy depth guard.
- **`parser/`** — fault-tolerant tokenizer + lightweight AST with error
  collection, recovery points, progress guards, and panic-safe partial
  results. Covers modern syntax through PHP 8.5: enums, readonly, attributes,
  property hooks (8.4), asymmetric visibility, pipe operator `|>` (8.5).

### Delivered: protocol surface

Completion, hover, definition, **type definition**, **implementation**,
references, document symbols, **document highlight**, **folding ranges**,
**workspace symbols**, signature help, rename with prepare, inlay hints
(variable/foreach/closure-return/return types, parameter names), and code
actions: add missing import, remove unused import, organize imports, generate
constructor, generate getters/setters, implement missing methods, extract
variable, inline variable, copy namespace, move to namespace.

### Delivered: diagnostics

Native checks with stable codes, per-rule enable/disable via config, and a
soft mode that suppresses unknown-* findings while the index warms up:

`syntax-error`, `unknown-class`, `unknown-function`, `unknown-member`,
`builtin-unavailable` (PHP version/extension-profile aware), `unused-import`,
`unused-private-method`, `unused-private-property`, `unreachable-code`,
`redundant-nullsafe`, `redundant-union-member`, `invalid-builder-arg`
(+ `unknown-column` / `unknown-relation` on Eloquent builder chains).

External tools stay isolated: PHPStan and Laravel Pint run on save with
per-file result caching. Formatting remains fully delegated to Pint.

Builtins are profile-driven: a generated name registry plus layered embedded
stubs (PHP 7.4 baseline + version deltas + extensions), selected from
composer platform data, `require.php`, and local PHP detection.

### Delivered: framework intelligence

- **Laravel** — route names (completion/definition), view names, translation
  keys, config/env key completion with dot notation, container bindings
  (defaults + provider scanning, facade resolution), Eloquent relations /
  accessors / casts / scopes with generic-aware builder returns, migration-
  derived schema, optional live DB introspection (MySQL/PostgreSQL/SQLite),
  composer.json hover/definition cards.
- **Symfony** — routes from PHP attributes plus YAML/XML config, service IDs
  from `services.yaml`/`services.php` plus defaults, Doctrine entities
  (attribute and annotation syntax: columns, relations, type mapping).

### Delivered: beyond the editor

- **MCP server** (`tusk-mcp`) — 13 tools (symbol search/explain/references,
  diagnostics, DB schema queries, Laravel routes/models, reindex) plus JSON
  resources and a `dump` mode that writes an AI context pack. This realizes
  the original "AI agent integration" goal ahead of schedule.
- **Graph export CLI** (`tusk-php graph`) — `container`, `references`
  (tier 1 structural), and `models` graphs as JSON / Mermaid / DOT, with
  `--deps none|boundary|full` vendor handling and a versioned DTO
  (SchemaVersion 1).
- **`deps unused`** — unused composer-dependency candidates from the
  reference graph, Laravel auto-discovery aware.
- **`workspace/`** — headless bootstrap shared by LSP, MCP, and CLI, so all
  surfaces index identically.
- **WASM (`wasip1`) target** for vscode.dev; live DB introspection is the
  only feature gated off.
- **Quality infrastructure** — race-enabled tests with Codecov, a
  conformance/corpus suite (parser/index/operation invariants, determinism
  checks) running a PR tier and a nightly full-corpus tier, and three nightly
  fuzz targets.

## Product Direction (unchanged in spirit)

- Stay a broad, fast PHP LSP whose differentiator is **deep Laravel and
  Symfony intelligence**.
- VS Code, Zed, and Neovim are first-class targets; generic LSP clients after.
- The **AI-agent surface (MCP + graph CLI) is now a second product pillar**,
  not a future idea. Code intelligence built once in `workspace/` should be
  consumable by editors and agents equally.
- Foundational abstractions before feature batches, same as before.

### Non-goals (unchanged)

- No native formatter; Laravel Pint stays the formatting path.
- No whole-parser rewrite while the current architecture still extends
  cleanly.
- No fragile text-scanning features where a small foundational abstraction
  would make them reliable.

## Workstreams

Ordered roughly by priority. Items marked **[unblocked]** have all their
foundations in place today.

### 1. Finish the diagnostics baseline

The original Milestone 2 has three rules left, and all of them were waiting
on the scope collector — which now exists.

- **Undefined variable** diagnostics via `scope/`. **[unblocked]**
- **Unused variable** diagnostics via `scope/`. **[unblocked]**
- **Argument count mismatch** via callable resolution + `ParamDef`.
  **[unblocked]**
- **Per-rule severity configuration.** Rules can be toggled but severities
  are fixed; the config surface (`diagnosticRules`) should accept
  `"off" | "hint" | "info" | "warning" | "error"` per code.
- **Pull diagnostics** (`textDocument/diagnostic`, LSP 3.17) alongside the
  existing publish path, useful for both modern editors and agent loops.

### 2. Parser and type-model deepening

The parser handles modern syntax at the declaration level; the remaining
gaps are inside expressions and in PHPDoc semantics.

- **Structured expression coverage:** `match` arms, named arguments, and
  first-class callable syntax (`strlen(...)`) are currently token-level only.
  These block better signature help, argument-count checks, and graph tier 2.
- **PHPDoc tags not yet modeled:** `@extends` / `@implements` (generic
  inheritance), `@mixin`, and evaluated conditional return types (currently
  parsed but degraded to a union).
- **Compat-layer retirement:** migrate remaining `FileNode`/`ClassNode`
  consumers to `ParseResult`/`ClassDef` (the compat conversion already drops
  data, e.g. `EndLine`), then delete `compat.go`.
- Keep the rule: fault tolerance is non-negotiable — bail-outs, progress
  guards, partial results on panic.

### 3. Builtin stub pipeline maturation

- **Run the phpstorm-stubs availability generator** (pending license review)
  to replace the hand-seeded `@since` table, then prune redundant
  hand-authored entries.
- **Richer per-version stub deltas** (8.1–8.5 core, more extensions), which
  also unlocks `builtin-unavailable` **method-signature deltas** (e.g.
  `DateTimeImmutable::createFromInterface()` on PHP < 8.0).
- **`.tusk-php.json` `phpVersion`/`extensions` override** slotted into the
  resolution chain between composer `require.php` and local PHP detection.

### 4. Protocol surface, round 2

What a mature LSP server still lacks here, in value order:

- **Fix the `executeCommand` advertisement mismatch** — `copyNamespace` and
  `moveToNamespace` are handled but not advertised (spec violation; quick).
- **Semantic tokens** — the single biggest visible upgrade for editors;
  the token stream already exists in the parser.
- **Selection range** (cheap on top of token/AST ranges).
- **Call hierarchy** — depends on tier-2 call edges (workstream 6); design
  them together so the LSP feature and the graph share one derivation.
- **Type hierarchy** — the index already has `reverseImplementsMap`-style
  data; mostly protocol plumbing.
- **`completionItem/resolve`** for lazy documentation, trimming completion
  payloads.
- Code lens stays deferred until reference counting is cheap enough.

### 5. Framework depth, round 2

- **Symfony parity items:** translation keys, `#[Autowire]` /
  `#[AsCommand]` / tagged-service attribute parsing, console command
  discovery, autowiring diagnostics.
- **Laravel:** model factory awareness, request validation rule completion,
  richer route handler resolution (controller arrays, closures).
- **Blade investigation** — component/slot/directive completion; decide
  whether Blade is in scope for the server or for editor extensions.
- **Twig investigation** — same decision for Symfony.
- Framework logic continues to live under `framework/laravel/` and
  `framework/symfony/`, never inside generic completion.

### 6. Graph and analysis, tier 2

The structural (tier-1) graph shipped; the deferred tier needs the expression
parsing from workstream 2.

- **Call/expression edges** (`$obj->method()` dispatch with container-aware
  interface→concrete resolution).
- **Tracing** — depth-bounded static call tree from an entry point (route,
  command, listener); flagship CLI output before any LSP integration.
- **Precise dead-code detection** — reachability from real entry points.
- **Models ERD detail** — column/cast nodes from the schema cache.
- Near-term graph fixes: Doctrine XML-mapped and string-form
  `targetEntity` relations; Symfony `config/bundles.php` exclusion in
  `deps unused`.

### 7. AI-agent surface maturation

The MCP server exists; make it excellent.

- Expose **Symfony routes/services** as MCP tools (Laravel already has
  parity tools).
- **Graph access over MCP** (container/references/models as tool results),
  so agents don't need to shell out to the CLI.
- Stabilize and document the tool contracts + `dump` context-pack format as
  a versioned interface, like the graph DTO.
- Honor the existing `ai.mcp` / `ai.writeTools` / `ai.allowedRoots` gates as
  the security boundary for any future write-capable tools.

## Housekeeping Backlog

Open low-severity items tracked in `CURRENT_ISSUES.md` (kept there with full
detail; listed here so the roadmap reflects reality):

- L26 — Doctrine XML mappings / string-form `targetEntity` missing from
  `graph models`.
- L25 — Symfony `bundles.php` exclusion for `deps unused`.
- L24 — `ResolveClassName` bare-short-name fallback leaking into
  `graph references --deps full`.
- L23 — compat `ClassNode` lacks `EndLine`.
- L21 — test seam for injecting container bindings.
- L20 — duplicate `installed.json` unmarshal structs in `composer/`.
- L19 — protocol-harness settle synchronization for diagnostics re-publish.
- L17 — `.tusk-php.json` PHP-version override (also workstream 3).
- L16 — `builtin-unavailable` signature deltas (also workstream 3).
- L14 — prune hand-authored availability entries post-generator.

## Suggested Delivery Order

1. **Quick wins:** executeCommand advertisement fix; per-rule severity
   config; L20/L23 cleanups.
2. **Diagnostics completion:** undefined/unused variable, argument count —
   closes the original Milestone 2.
3. **Stub pipeline:** phpstorm-stubs generation + `.tusk-php.json` override.
4. **Expression parsing investment:** match/named-args/first-class callables
   structured in the AST — this one piece unblocks signature-help precision,
   argument-count accuracy, call hierarchy, and graph tier 2 simultaneously.
5. **Semantic tokens + selection range.**
6. **Graph tier 2 + call hierarchy** (designed together).
7. **Framework round 2** (Symfony parity first, then Blade/Twig decisions).
8. **MCP maturation** in parallel with 6–7, since it mostly re-exposes
   existing analysis.

## Quality Bar

Unchanged, and now enforced by infrastructure:

- Capabilities are advertised only when implemented; every feature gets
  handler tests, provider-level unit tests, and framework fixtures where
  relevant.
- Diagnostics use stable codes, the narrowest reliable range, and are
  individually configurable; fast checks never block editing; expensive
  checks run on save or debounced.
- Code actions return deterministic `WorkspaceEdit`s and never touch
  unrelated code.
- Parser changes must keep the conformance invariants green (never-nil,
  no-panic, deterministic, idempotent indexing) and survive the nightly
  corpus + fuzz runs.
- Versioned external contracts (graph DTO, MCP tools, dump format) evolve by
  schema version, not silent breakage.
