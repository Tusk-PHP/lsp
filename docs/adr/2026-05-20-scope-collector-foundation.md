# ADR: Initial Reusable Scope Collector Foundation

## Status

Implemented for Sprint 1 roadmap foundation work on 2026-05-20.

## Context

`ROADMAP.md` calls for a reusable scope collector under `internal/scope` to
support undefined/unused variable diagnostics, scoped rename, safer
references, closure parameter inference, and better inlay hints.

Before this change, local variable handling in `internal/analyzer` was bounded
by simple line scanning inside the nearest enclosing method. That worked for
basic local variables but did not provide a shared abstraction for parameters,
`foreach`, `catch`, destructuring, closure imports, or top-level function
scopes.

## Decision

Add a new `internal/scope` package with a token-based collector that produces a
document-level scope model:

- `Document`
- `Scope`
- `Binding`

The collector tracks:

- file scope
- function, method, and closure scopes
- parameters
- first-assignment local variables
- `foreach` key/value bindings
- `catch` bindings
- destructuring bindings
- closure `use (...)` imports
- basic top-level `use` aliases

Bindings record declaration ranges plus reference ranges. Closure-imported
bindings can point at an `Origin` binding so grouped occurrences can be queried
from either side of the closure boundary.

## Implementation

Implemented files:

- `internal/scope/collector.go`
- `internal/scope/collector_test.go`

Analyzer integration:

- `internal/analyzer/analyzer.go`

Focused analyzer coverage:

- `internal/analyzer/references_test.go`
- `internal/analyzer/rename_test.go`

The analyzer now routes local variable/parameter/foreach-style reference and
rename operations through `internal/scope` instead of the older method-bounded
line scan.

## Tradeoffs

Why token-based first:

- It matches the project's lightweight parser strategy.
- It stays reusable across analyzer, diagnostics, and inlay-hint work.
- It avoids baking feature-specific line heuristics into more packages.

What this deliberately does not solve yet:

- full arrow-function capture modeling
- grouped `use Foo\{Bar, Baz};` alias collection
- deeper expression-level data flow
- block-level lifetime analysis for future unused-variable diagnostics

This is an intentional first slice: enough structure to replace the most
fragile analyzer logic without trying to become a full semantic engine in one
step.

## Results

Added collector tests covering:

- local variables
- parameters
- `foreach` bindings
- `catch` bindings
- closure `use (...)`
- basic import aliases

Added analyzer tests covering:

- parameter references
- `foreach` key rename

## Remaining Work

Likely next steps:

1. Reuse the collector in diagnostics and inlay hints.
2. Add arrow-function capture support.
3. Extend import handling for grouped `use` statements.
4. Expose scope lookup helpers needed by the shared cursor-context layer.
