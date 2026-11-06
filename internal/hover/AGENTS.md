# internal/hover

LSP `textDocument/hover` provider.

## Entry points

- `Provider.Hover()` in `provider.go` — main entry.
- `format.go` — renders the markdown that ends up in the client.
- php.net manual links for builtins come from `symbols.PHPManualURL` (`internal/symbols/manuallinks.go`); hover only renders them.

## What hover does

Resolves the symbol under the cursor and formats it. Hover handles:

- variables (uses `@var` annotations, then `resolve/` for chain-derived types),
- property/method access chains (`$this->logger->info` is resolved step-by-step through the symbol index),
- class/interface/trait names,
- `self`/`static`/`parent`,
- Laravel facades (resolves to the concrete bound class and its docblock-declared methods),
- array keys including nested literal/PHPDoc shapes,
- config keys (Laravel) — leaf values render inline.

## Conventions

- Type resolution goes through `internal/resolve/` (don't reparse PHPDoc here). If you need a new type form, add it to `resolve/` and consume it.
- Generic substitution is applied in `format.go` via the `resolve/templates.go` mapping — keep substitution at format time, not at lookup time.
- Hover must be cheap — never block on PHPStan/Pint or anything from `diagnostics/`.
- New behavior gets a `*_test.go` next to the existing pattern (`facade_test.go`, `arraykeys_test.go`, `generics_coverage_test.go`).

The project-wide rules in the root [AGENTS.md](../../AGENTS.md) apply here too.
