# Sprint 4 Code Actions Server/Handoff

Date: 2026-05-21
Scope: Worker C server capability coverage, codeAction request plumbing, editor verification handoff

## Decision

Treat Sprint 4 editor-facing code action support as two distinct contracts on the server side:

1. `initialize` must advertise the granular kinds editors use to route UI affordances:
   - `quickfix`
   - `refactor`
   - `refactor.move`
   - `source`
   - `source.organizeImports`
2. `textDocument/codeAction` must honor `context.only` filtering even when analyzer-side
   action generation is broader than the editor request.

## Finished In This Slice

- `internal/lsp/server.go`
  - Expanded `CodeActionOptions.codeActionKinds` to advertise `quickfix` and
    `source.organizeImports` alongside the existing broader kinds.
  - Added server-side `context.only` filtering using LSP kind-prefix matching
    so `source` matches `source.*` and `refactor` matches `refactor.*`.

- `internal/lsp/server_test.go`
  - Tightened initialize assertions to verify the advertised code action kinds.

- `internal/lsp/protocol_harness_test.go`
  - Added protocol-level coverage asserting that `quickfix` and
    `source.organizeImports` are present in the real initialize response.

- `internal/lsp/coverage_test.go`
  - Added an end-to-end `textDocument/codeAction` test proving that requests
    carrying both `context.diagnostics` and `context.only` are accepted and that
    the server filters returned actions by requested kind.
  - Current regression shape:
    - `only: ["source"]` returns only `source` / `source.*`
    - `only: ["refactor"]` returns only `refactor` / `refactor.*`
    - `only: ["quickfix"]` currently returns no actions until analyzer quickfix
      generation lands

## Editor Metadata

No editor package manifest changes were required in this slice.

VS Code, Zed, and Neovim all discover code action support from the server's
runtime `initialize` response rather than from static extension metadata in
this repository. The relevant change is therefore the server capability payload.

## Manual Verification Status

Manual verification was not run in this environment.

Reason:
- This workspace provides source access and CLI test execution, but not an
  interactive GUI/editor session for VS Code or Zed.
- Neovim could be driven here only if the local config already includes an LSP
  client setup for this repository; that setup is environment-specific and not
  versioned in this repo.

## Reproducible Verification Steps

### VS Code

1. Build/install the extension from `editors/vscode`.
2. Launch VS Code with this repository open.
3. Open a PHP file containing:
   - an unused `use` import
   - an unknown class reference
4. Run `Developer: Toggle Developer Tools` and confirm the client/server
   initialize exchange advertises:
   - `quickfix`
   - `source.organizeImports`
5. Trigger code actions on the unused import line:
   - lightbulb should show a quick fix once analyzer quickfix generation is present
6. Run `Organize Imports` from the command palette:
   - the request should send `context.only = ["source.organizeImports"]`
7. Confirm the server response contains only organize-import actions for that
   request.

### Zed

1. Install/load the `editors/zed` extension for this repository.
2. Open the same PHP fixture file.
3. Use Zed's code action UI on the unused import and unknown class locations.
4. Confirm the server advertises `quickfix` and `source.organizeImports` in the
   initialize handshake from Zed's LSP logs.
5. Trigger organize imports and verify the outgoing request contains
   `only: ["source.organizeImports"]`.

### Neovim

1. Configure this server in your local LSP setup and open the repository.
2. Open a PHP file with an unused import and an unknown class.
3. Inspect capabilities with `:lua =vim.lsp.get_clients()[1].server_capabilities.codeActionProvider`.
4. Confirm `codeActionKinds` includes `quickfix` and `source.organizeImports`.
5. Invoke:
   - `vim.lsp.buf.code_action({ context = { only = { "quickfix" } } })`
   - `vim.lsp.buf.code_action({ context = { only = { "source.organizeImports" } } })`
6. Verify the quickfix request stays scoped to quickfix actions and the organize
   imports request stays scoped to source organize-import actions.

## Still Open

- Analyzer-side generation for:
  - remove unused import quickfix
  - add missing import quickfix
  - source organize imports action
- Cross-editor manual verification after those actions land

## Router Next

Once Worker A/B land analyzer actions, re-run the editor verification steps
above and update only if a client-specific mismatch appears.
