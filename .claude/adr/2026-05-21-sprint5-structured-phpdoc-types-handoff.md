# Sprint 5 Structured PHPDoc Types Handoff

## Context

Sprint 5 calls for a shared structured PHPDoc type model, alias support, imported aliases, conditional return parsing, and first wiring through the shared completion/hover path.

## Changes

- Extended `internal/types` from string helpers into a small structured PHPDoc type model that can round-trip:
  - unions
  - intersections
  - generics
  - array/list/object shapes
  - `T[]` array suffixes
  - callable signatures
  - scalar/string/null literals
  - conditional types
- Kept the existing string helpers (`ParseArrayShape`, `ExtractDocTypeString`) so current callers stay compatible.
- Added parsed type fields to `parser.DocBlock` members:
  - `DocParam.ParsedType`
  - `DocReturn.ParsedType`
  - `DocProperty.ParsedType`
  - `DocMethod.ParsedType`
  - `DocTemplate.ParsedBound`
- Added structured parsing for:
  - `@phpstan-type` / `@psalm-type`
  - `@phpstan-import-type` / `@psalm-import-type`
- Added symbol-index storage for class-scoped/imported type aliases and resolver-side alias expansion.
- Updated the shared resolver to derive `ResolvedType` from the structured PHPDoc model instead of only string-splitting generic syntax.
- Alias-expanded method/property doc types now feed the shared resolver paths used by hover and completion.

## Decisions

- I kept the existing raw string fields on doc structures rather than replacing them. That keeps current code stable while allowing newer consumers to opt into structured data.
- `ResolvedType` still remains intentionally smaller than the PHPDoc AST. For unsupported semantic cases like conditional types, the resolver picks the most useful branch for hover/completion instead of trying to become a full static analyzer.
- Alias expansion stays class-scoped and lazy through the symbol index. That matches current ownership boundaries and keeps cross-class imported alias lookup out of the parser.

## Tests

Ran with `GOCACHE=/Users/d8vjork/Projects/OpenSoutheners/php-lsp/.gocache`:

- `go test ./internal/types ./internal/parser ./internal/resolve ./internal/symbols`
- `go test ./internal/hover ./internal/completion`

## Handoff Notes

- `CURRENT_ISSUES.md` was left unchanged. I did not defer any non-critical follow-up from this slice.
- The workspace already contains unrelated user-owned files and generated artifacts (`CURRENT_ISSUES.md`, `ROADMAP.md`, `PHPANTOM_COMPARISON.md`, `.gocache/`). Do not revert them.
- The next natural follow-up is to move more call sites from raw doc strings to `ParsedType` directly, especially any remaining PHPDoc-specialized code in hover/completion that still reparses strings ad hoc.
