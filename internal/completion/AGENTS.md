# internal/completion

LSP `textDocument/completion` provider.

## Entry points

- `Provider.Complete()` in `provider.go` — main entry. Detects cursor context (`->`, `::`, `$`, `new`, `use`, function call, attribute, etc.) and dispatches.
- `autoimport.go` — inserts missing `use` statements via additional text edits.

## Specialized completion sources

Each file is a focused context with its own tests:

- `arraykeys.go` — array-shape keys from PHPDoc / literals.
- `columns.go` — Eloquent builder columns (driven by `internal/models/eloquent.go`).
- `config.go` — Laravel config keys.
- `container.go` — `app()`, `resolve()`, `$this->app->make()` argument completion driven by `internal/container/`.
- `laravel_routes.go` — route name completion (`route('...')`).
- (Views/translations live under `internal/framework/laravel/` and are consumed here.)

## Conventions

- Cursor classification should move to `internal/source/` over time. New completion contexts that need to inspect cursor state should prefer `source.CursorContext` instead of re-scanning the line.
- Type resolution for the receiver of `->` / `::` goes through `internal/resolve/`. Don't reparse types here.
- Generic inference for chain returns (`Collection<T>::first(): T`) is exercised by `chain_resolution_test.go`, `variable_generics_test.go`, and `e2e_generics_test.go` — preserve those when changing resolution.
- `e2e_*_test.go` files exercise the provider end-to-end with framework fixtures; add an e2e case when shipping a framework-aware completion.
- Auto-import edits must be additional `TextEdit`s on the completion item, not a separate command — clients won't always run commands.

The project-wide rules in the root [AGENTS.md](../../AGENTS.md) apply here too.
