# ADR: Parser Error Collection For Sprint 1

## Status

Implemented in a minimal compatibility-preserving form on 2026-05-20.

## Context

`ROADMAP.md` calls out parser error collection as Sprint 1 foundation work for
syntax diagnostics. The current parser already accumulates recoverable syntax
errors in `ParseResult.Errors` from two places:

- tokenizer errors such as unterminated strings and comments
- structural recovery errors such as missing `)` and `}` during lightweight AST
  extraction

That collection was not available through the legacy-compatible `ParseFile`
entrypoint. `ParseFile` converted `ParseResult` into `FileNode` and dropped the
error slice entirely. Most analyzer, completion, hover, symbol, and diagnostics
paths still call `ParseFile`, so the omission made parser errors hard to consume
without bypassing the compatibility layer.

## Decision

Expose parser errors on `FileNode` and copy `ParseResult.Errors` into that field
inside `toFileNode`.

This is intentionally small:

- no change to `ParseResult`
- no change to parser recovery behavior
- no change to existing `ParseFile` call sites
- no new diagnostics wiring outside `internal/parser`

## Implementation

Changed:

- `internal/parser/compat.go`
  - added `Errors []ParseError` to `FileNode`
  - updated `toFileNode` to copy `ParseResult.Errors`
- `internal/parser/parser_test.go`
  - added coverage for `ParseFile` exposing tokenizer errors
  - added coverage for `ParseFile` exposing structural recovery errors
  - tightened the existing valid-parse wrapper test to assert zero file errors

## Compatibility

This is non-breaking for current callers:

- existing `ParseFile` consumers continue to compile unchanged
- parser tests that assert `ParseResult.Errors` still behave the same
- analyzer tests continue to pass because `FileNode` shape only grew by one
  optional field

## Tradeoffs

Pros:

- makes syntax-error collection reachable from the main compatibility API
- keeps Sprint 1 progress scoped to `internal/parser`
- avoids forcing parser/analyzer consumers to switch entrypoints

Cons:

- `FileNode` now carries both structural AST data and parser errors
- there is still no dedicated diagnostics-oriented parser API
- downstream packages still need follow-up work to actively publish these
  errors as diagnostics

## Deferred

The following was intentionally deferred:

- introducing a separate `ParseFileWithResult` or diagnostics-specific parser
  facade
- threading parser errors into `internal/diagnostics`
- richer error metadata beyond message, line, and column
- deduplication, severity, or recovery-class tagging for parse errors

## Remaining Work

1. Teach the diagnostics layer to consume `FileNode.Errors` or `ParseResult.Errors`
   and publish syntax diagnostics.
2. Decide whether `FileNode.Errors` is the long-term compatibility surface or an
   interim bridge before a cleaner parser/diagnostics interface exists.
3. Add protocol-level diagnostics tests once syntax diagnostics are wired into
   the LSP server.
