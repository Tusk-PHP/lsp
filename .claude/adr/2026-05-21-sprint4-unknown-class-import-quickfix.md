# Sprint 4 Unknown-Class Import Quick Fix

## Context

Worker B owned the unknown-class quick-fix path for Sprint 4. The repo already had parallel work in progress for unused-import quick fixes and organize-imports source actions, so this change stays on the symbol lookup and unknown-class diagnostic path and only touches shared code-action plumbing where required.

## Decision

Add deterministic `quickfix` code actions for diagnostics with code `unknown-class` by:

- reading the unresolved class reference from the diagnostic range
- resolving candidate class-like symbols from the workspace index by short name
- narrowing qualified references by FQN suffix when the unresolved text already contains namespace segments
- returning one action per deterministic candidate, sorted by FQN
- marking the action preferred only when there is exactly one candidate

The quick fix inserts `use Candidate\FQN;` and, when safe, also replaces the unresolved qualified reference with the imported short name.

## Safety Rules

- No alias guessing: if the target short name conflicts with an existing class import alias or a locally declared class/interface/trait/enum name, no action is returned for that candidate.
- No ambiguity collapse: when multiple indexed symbols share the same short name, multiple actions are returned in sorted order instead of guessing.
- Short-name replacement only happens when the unresolved text already contains namespace separators. Plain `new Logger()` only gets the import edit.

## Implementation

- Added `internal/analyzer/code_actions.go` for code-action filtering, unknown-class candidate lookup, import edit building, and safe shortening logic.
- Updated `Analyzer.GetCodeActions` to:
  - start from an empty slice instead of `nil`
  - add unknown-class quick fixes
  - respect `context.only` for `quickfix`, `source`, and `refactor.move`
- Added analyzer tests covering:
  - single-candidate import quick fix
  - deterministic multiple candidates
  - qualified-reference shortening
  - alias-conflict suppression
  - `only=quickfix` filtering
- Added an LSP harness test that verifies the JSON-RPC `textDocument/codeAction` response includes the expected quick fix edit.

## Verification

- `go test ./internal/analyzer ./internal/lsp ./internal/diagnostics`

## Handoff Notes

- Manual editor verification in VS Code, Zed, and Neovim was not run in this session. The LSP test covers server response shape, but client UX still needs the sprint verification pass.
- The import insertion logic intentionally does not sort or regroup imports. That stays with the organize-imports source action work.
