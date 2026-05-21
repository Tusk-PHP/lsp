# Sprint 5: Conditional Doc Types in Hover and Completion

Date: 2026-05-21

## Scope

Worker C owned the conditional PHPDoc return-type slice and the hover/completion
display wiring for the shared structured type model.

## Changes

1. Added a structured PHPDoc type model in `internal/types/types.go`.
   - Parses unions, intersections, generics, array/object/list shapes,
     array-suffix types, callable signatures, string literals, and
     conditional types.
   - Normalizes rendering through `TypeExpr.String()` / `RenderTypeExpr(...)`.
   - Added `Parse(...)` and `ExpandAliases(...)` so the existing alias-aware
     resolver path can consume the same model.

2. Fixed docblock type extraction for conditional expressions.
   - `ExtractDocTypeString(...)` now keeps scanning across top-level spaces when
     they are part of the type expression, which allows:
     `($id is class-string<T> ? T : object)`.

3. Wired shared structured output into hover and completion display surfaces.
   - `internal/hover/format.go` now renders parameter, return, property, and
     throw types via `internal/types`.
   - `internal/completion/provider.go` now renders completion param/return
     detail strings and `@var` fallback types via `internal/types`.

4. Restored the alias-aware doc-type resolver shim expected by the current
   resolver work in progress.
   - Added `internal/resolve/doc_types.go` to bridge parsed doc types,
     imported aliases, and `ResolvedType`.

## Tests

Ran:

`env GOCACHE=/private/tmp/php-lsp-gocache go test ./internal/types ./internal/parser ./internal/hover ./internal/completion ./internal/resolve`

Added focused coverage for:

- conditional docblock return parsing
- conditional hover rendering
- conditional completion detail rendering
- structured type rendering / extraction in `internal/types`

## Decisions

- Kept this slice display-oriented. Conditional types are parsed and rendered
  consistently for hover/completion, but there is no attempt here to evaluate
  the runtime truthiness of the condition.
- Reused the same structured type model for alias expansion so the resolver can
  keep moving toward a shared doc-type pipeline instead of adding more string
  rewriting.

## Handoff Notes

- The worktree already contains unrelated Sprint 5 alias/resolver changes in
  other files (`internal/parser/compat.go`, `internal/resolve/{resolve,types,templates}.go`,
  `internal/symbols/index.go`, etc.). This slice was implemented to compile and
  test against that in-progress state without reverting it.
- I did not append anything to `CURRENT_ISSUES.md`; no new deferred issue was
  necessary from this slice after the focused tests passed.
