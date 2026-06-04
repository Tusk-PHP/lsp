# Design: Graph Export CLI

Status: Proposed
Owner: @d8vjork
Last updated: 2026-06-04

## Summary

Expose the server's existing project analysis (symbol index, DI container,
Eloquent/Doctrine models, class hierarchy) as a **graph export CLI** that emits
versioned JSON plus ready-to-render Mermaid and Graphviz DOT. The goal is to
turn data the LSP already computes into architecture visualizations: the
framework container, models as business-logic entities and their connections,
and class hierarchies.

The visualization frontend is intentionally **out of scope for this repo** — it
consumes the JSON contract and lives in a separate repository.

## Motivation

`cmd/tusk-php --parse <file>` currently dumps a single file's `parser.ParseResult`
(syntax). That is the wrong layer for the visualizations we want. The interesting
graphs are project-level and already exist above the parser:

- **Container** — `container.ServiceBinding{Abstract, Concrete, Singleton, Tags,
  Alias}` via `ContainerAnalyzer.GetBindings()`, plus
  `AnalyzeConstructorInjection()` → `InjectedDependency{ParamName, TypeHint,
  ResolvedConcrete}`. This is already a node/edge graph.
- **Models** — `models` injects Eloquent + Doctrine relations as virtual
  properties (relation kind in `DocComment`, related model in `Type`), and
  exposes `Schema`/`SchemaTable`/`SchemaColumn` for ERD-style output.
- **Class hierarchy** — `symbols.Symbol` carries `Extends` / `Implements` /
  `ParentFQN` across the whole index.

Because these all require the full pipeline (index + composer platform detection
+ container analyzer + models analyzer), the parser alone cannot produce them.

## Non-goals

- Building the visualization UI (separate repo, consumes JSON).
- Bundle-size accounting. Backend dependencies are weighted by **coupling**, not
  bytes (see Dependency handling).
- Replacing `--parse`. It stays as the file-level AST escape hatch.

## Repository strategy

**Decision: keep one Go module now. Do not split the parser into its own repo.**

Rationale:

- The viz tool barely depends on the parser; it depends on the analysis core
  (index/container/models). Extracting only the parser would not let an external
  consumer build a container graph.
- The real seam is *"PHP-project-model core"* vs *thin adapters* on top. We
  already have two adapters over that core (`cmd/tusk-php` LSP, `cmd/tusk-mcp`).
  The graph export is a third adapter and belongs alongside them.
- The analysis packages live under `internal/`, so Go forbids external import
  anyway. Splitting now would force promoting a public API surface during the
  period of highest churn.

**The JSON graph schema is the API boundary, not the package boundary.** Get the
DTO right and versioned, and code can move between repos later without consumers
noticing.

Resulting layout: **two repos, not three** — this Go repo (server + graph
export) and a separate visualization repo (different toolchain/cadence, consumes
JSON over a file or stdin).

### When to extract the parser/core later

Only when (a) a second real consumer outside this repo needs it, **and** (b) the
export DTOs have stabilized. Until both hold, multi-repo overhead (replace
directives, lockstep version bumps, split CI) is pure tax with no realized reuse.

## Architecture

### 1. `graph` package

A new internal package that turns the live index/container/models into versioned,
serializable DTOs — **not** raw internal structs (so the frontend never couples
to Go internals).

```
type Graph struct {
    SchemaVersion int    `json:"schemaVersion"`
    Kind          string `json:"kind"`   // "container" | "models" | "classes"
    Nodes         []Node `json:"nodes"`
    Edges         []Edge `json:"edges"`
}

type Node struct {
    ID    string         `json:"id"`     // stable, e.g. FQN or package name
    Kind  string         `json:"kind"`   // class | model | binding | dependency-boundary
    Label string         `json:"label"`
    Meta  map[string]any `json:"meta,omitempty"`
}

type Edge struct {
    From string `json:"from"`
    To   string `json:"to"`
    Kind string `json:"kind"`            // extends | implements | injects | binds | hasMany | ...
}
```

### 2. Graph builders

All three build over the same fully-indexed project (reuse the LSP init path:
config, composer platform, container analyze, models analyze).

- **container** — nodes for bindings, edges for constructor-injection.
- **models** — nodes for entities, edges for relations; optional schema/ERD meta.
- **classes** — nodes for classes/interfaces/traits, edges for
  extends/implements.

### 3. Dependency handling (opt-in)

First-party vs dependency vs builtin is a single field check: `symbols.Symbol`
carries `Source ∈ {SourceProject, SourceVendor, SourceBuiltin}`. Mapping a vendor
FQN → its composer package reuses `composer` package data (`installed.json`
packages + PSR-4 prefixes, with best-prefix FQN resolution already implemented).

Flag `--deps`:

- `none` (**default**) — first-party only; vendor edges dropped.
- `boundary` — collapse each dependency to **one node per composer package**.
  No vendor internals. This is the primary mode.
- `full` — expand vendor too. Escape hatch, off by default.

**Boundary node weight = coupling, not size.** Backend dependencies have no
bundle cost; the meaningful signal is how tightly first-party code is coupled to
a package. Each `dependency-boundary` node carries in `Meta`:

- `package` — e.g. `monolog/monolog`
- `version` — from `installed.json`
- `edgeCount` — distinct first-party → package edges
- `distinctSymbols` — distinct vendor symbols actually touched

This surfaces over-coupling to a vendor — the backend equivalent of "this
dependency is too heavy." Fall back to top-level namespace when a FQN cannot be
resolved to a package (generated/root-namespaced code).

### 4. Output formats

`--format`:

- `json` (default) — the versioned contract; the only stable interface.
- `mermaid` — renders in GitHub/Obsidian with zero frontend.
- `dot` — Graphviz.

Mermaid + DOT are derived from the same `Graph` DTO and ship **first**: they
validate the data and are immediately useful before any UI exists.

### 5. CLI surface

```
tusk-php graph <container|models|classes> [--deps=none|boundary|full] [--format=json|mermaid|dot] [--root=.]
tusk-php parse <file>          # existing file-level AST (unchanged)
```

Sits alongside the existing `--parse` flag and the `cmd/tusk-mcp` adapter.

## Build sequence

1. `graph` package + DTO schema (`SchemaVersion = 1`).
2. `graph container` builder over `container.GetBindings()` +
   `AnalyzeConstructorInjection()`; JSON output.
3. `--deps=boundary` with coupling weights.
4. Mermaid + DOT renderers.
5. `graph models` and `graph classes` builders.
6. (Separate repo) visualization frontend against the JSON contract.

## Derived value: the reference-graph layer

The container/models/classes graphs above are **structural** ("what contains or
extends what"). Three higher-value features need a different layer — a
**reference/call graph** ("what actually reaches what"). Adding a reference-edge
layer to the `graph` package yields all three. The raw material already exists:

- `analyzer.FindAllReferences` — working who-references-whom engine.
- `checks/{unused_private,unused_imports,unreachable}.go` — the sound,
  zero-false-positive slice of dead code.
- `framework/laravel` `RouteName{HandlerFQN, HandlerMethod}` and
  `framework/symfony` `DiscoverRoutes()` — real entry points with resolved
  controller targets, used to seed reachability roots and trace origins.

### Unused dependencies

A `dependency-boundary` node with **zero inbound first-party edges** is a
candidate unused package (the edge count is already computed for coupling
weight). Correctness rules — the whole value is false-positive control, and
framework awareness is the differentiator over `composer-unused`:

- Flag only **direct `require`** (read `composer.json`; ignore `require-dev` and
  transitive deps from `installed.json`).
- Exclude non-static usage the framework layer already knows about: Laravel
  auto-discovered providers (`extra.laravel.providers`), Symfony `bundles.php`,
  config-string/facade usage, and `ext-*` (functions, not classes).
- Label as **"no static reference found — review"**, never "unused."

### Dead code (tiered by confidence)

- **Sound tier (exists):** unused private members + unreachable code. Stays as
  editor diagnostics.
- **Heuristic tier (new):** *orphan* symbols — public methods/classes with zero
  inbound reference edges **after seeding roots from real entry points** (routes,
  console commands, event subscribers, container bindings). The route index is
  what stops every controller method from being flagged dead. Ships as an
  opt-in CLI report, not a diagnostic (magic methods, `$obj->$method()`, and
  reflection guarantee misses).

### Clear paths / tracing (flagship)

A new `trace` graph kind: a depth-bounded static call-tree rooted at one entry
point (route name, or any `Class::method`) — "drive through the codebase without
a debugger," scoped small enough to stay readable.

Differentiator: when a controller injects an **interface**, the `container`
analyzer knows the concrete binding and `resolve/` knows the types, so the trace
follows into the *real implementation* instead of dead-ending. Interface→concrete
resolution on a call path is the killer feature. Unresolvable/dynamic hops render
as explicit `⚠ dynamic` nodes rather than being dropped.

### Decisions

- **Output posture — both, separate surfaces.** Sound facts (unused private,
  unreachable) stay as editor diagnostics; heuristic candidates (orphan symbols,
  unused-package suspects) are a separate opt-in CLI report. Keeps "certain" and
  "worth a look" on distinct surfaces.
- **Trace surface — CLI export first.** Emit trace trees as Mermaid/JSON rooted
  at a route name or `Class::method`, validating the static-tracing engine before
  any editor (LSP code-lens/command) integration.

### Build sequence impact

Insert after step 4 (renderers): build the reference-graph layer, then
`graph trace` (flagship), then the `unused-deps` and `orphan` reports.

## Open questions

- ERD detail level for `models` (relations only vs full column schema) — gate
  behind a flag if schema introspection is expensive.
- Stable node IDs across runs (FQN is the natural key; confirm uniqueness for
  anonymous classes / closures).
