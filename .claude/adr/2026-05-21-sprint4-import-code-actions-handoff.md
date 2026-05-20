# Sprint 4 Import Code Actions Handoff

Date: 2026-05-21
Owner: Worker A
Scope: analyzer/LSP code actions for unused-import and organize-imports

## Decision

Keep Sprint 4 import actions native and conservative:

1. `unused-import` gets a deterministic `quickfix` that deletes exactly the diagnosed `use` line.
2. `source.organizeImports` rewrites only the contiguous top-level import block, removing unused imports and sorting the remaining imports by kind and name.
3. The organize action is offered only when the block is safe to rewrite: simple one-line imports, no grouped imports, and no comments or non-blank text interleaved inside the block.

## Implementation

- `internal/analyzer/analyzer.go`
  - `GetCodeActions` now appends:
    - unknown-class quickfixes from the existing shared path
    - unused-import quickfixes
    - `source.organizeImports`
  - Existing `source` and `refactor.move` actions now respect `context.only`.

- `internal/analyzer/code_actions.go`
  - Added `unusedImportQuickFixes`.
  - Added `organizeImportsAction`.
  - Added import-block safety and rendering helpers:
    - safe import block detection
    - deterministic sort/group rendering
    - line/block edit range helpers
  - Kept the organize rewrite text-preserving by reusing the original import lines instead of regenerating `use` statements from parsed metadata.

## Behavior

- Quickfix:
  - kind: `quickfix`
  - title: `Remove unused import`
  - edit: single whole-line deletion
  - diagnostics: includes the triggering `unused-import` diagnostic

- Organize imports:
  - kind: `source.organizeImports`
  - title: `Organize Imports`
  - removes unused imports
  - sorts groups as class, function, const
  - sorts within a group by FQN, then alias, then source text
  - inserts a blank line between groups

## Safety Limits

- No action is offered for grouped imports like `use Foo\{Bar, Baz};`.
- No organize action is offered when comments or other non-blank lines appear inside the import block.
- The quickfix still works for a simple unused import line even if organize-imports is skipped.

## Tests

- `internal/analyzer/code_actions_test.go`
  - unused-import quickfix
  - organize-imports rewrite
  - unsafe commented import block skipped
  - existing unknown-class quickfix coverage preserved

- `internal/lsp/coverage_test.go`
  - end-to-end `textDocument/codeAction` coverage for:
    - unused-import quickfix edit
    - organize-imports source action edit
    - `context.only` filtering

- Full verification run:
  - `go test ./internal/analyzer ./internal/lsp`
  - `go test ./...`

## Editor Verification Notes

I could not launch VS Code, Zed, or Neovim in this environment. The LSP-side verification for those clients is covered by:

- advertised code action kinds already present in server initialize tests
- end-to-end codeAction JSON shape tests in `internal/lsp`
- `context.only` filtering, which is the main interoperability requirement for editor-triggered organize-imports requests

## Handoff Concerns

- The parser surface still does not expose enough structure to safely rewrite grouped imports or preserve inline/commented import blocks; keep those cases intentionally unsupported unless parser metadata grows.
- If Sprint 4’s unknown-class import resolution work starts reorganizing import insertion logic, it may be worth consolidating the import safety helpers so quickfix insertion and organize-imports share one import-block model.
