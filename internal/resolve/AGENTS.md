# internal/resolve

Shared type and symbol resolution. Hover, completion, diagnostics, and the analyzer should all agree on types by going through this package.

## Entry points

- `Engine` and `Resolver` (`resolve.go`) — an `Engine` per provider owns the `*symbols.Index`, the hooks and the caches; `Engine.Resolver()` starts one `Resolver` per request, which resolves expressions/types in context and carries that request's recursion guard. Take one `Resolver` at the top of a request and pass it down; never keep one in a field or share it between goroutines. A hook (`ChainResolver`, `TypedChainResolver`, `ContainerCall`) must resolve through the `Resolver` it's handed, or a cycle through it escapes the guard and overflows the stack.
- `ResolvedType` (`types.go`) — `{FQN, Params[]}` plus the structured parts: `Union`, `Intersection`, `Callable`, shape `Fields`, `Literal` and `Conditional`. For a union, `FQN`/`Params` mirror its most useful part, so single-class readers keep working; `FullString()` shows all of it, `MemberOwner()` gives every class to look members up on. The canonical type representation everywhere downstream.
- `ParseGenericType(raw)` — produces a `ResolvedType` from a PHPDoc type string.
- Generic classes are typed from their own PHPDoc (`@template`, `@extends`, `static<…>`). `templates.go` reads the index's template fallbacks (`symbols.Index.SetTemplateFallbacks`, set by a framework such as `framework/laravel`) only for a class whose PHPDoc declares no `@template`; resolve itself knows no framework class.
- `FindEnclosingClass(file, pos)` / `FindEnclosingMethod(file, pos)` — scope helpers used by every provider.
- PHPDoc types are parsed by `internal/types` and read into `ResolvedType` through `parseType` (`parse_cache.go`) and `resolveDocType` (`resolve.go`).
- `type_aliases.go` — `@phpstan-type` / `@phpstan-import-type` aliases.

### Generics engine (PHPStan semantics, issue #68)

- `generics.go` — `applyEnv` (substitute templates and `static`, resolve names, evaluate `key-of`/`value-of`/`T[K]`/`new<T>`), `bindTemplates` (match a parameter's declared type against an argument's type), `AncestorTypeArgs` (`@extends`/`@implements`/`@use` arguments through the hierarchy), `fillUnbound` (default, then bound, then `mixed`), `IterableTypes`.
- `call.go` — `MemberTypeForCall` / `FunctionTypeForCall`: what a member or function gives at a call, with the receiver's and the call's templates bound and conditional return types decided. `MemberTypeResolved` is the same without arguments.
- `member_types.go` — `rawMemberReturnType` / `preferDocType`: when PHPDoc wins over the native type.
- `syntax_tree.go`, `expression.go` — `typeOf`: the one typer every feature types an expression with, from the file's syntax tree (`FileNode.Tree`, kept per source with the scopes): variables, literals, `new`, `::class`, arrays, closures, function calls, calling a closure, `->`/`::` chains with `$this`/`self`/`static`/`parent`, offsets, `??`, `?:`, ternaries, operators (`parser.OperatorType`, shared with the index's literal typer; `++`/`--` in `increment.go`) and container calls through the `ContainerCall` hook. `ExpressionType` and `ChainType` take expression text from callers that only have text and parse it as one expression. Don't add another walker in a feature; extend this one.
- `chain.go` — `ChainType`, and `ReceiverAt` / `ReceiverTypeAt` / `CallTypeAt`: the member access at the cursor, found in the file's tree, and the type of its receiver or call. A resolver wired to another through `TypedChainResolver` asks that one, so the providers share the completion provider's resolver, which knows the container.
- `access_chain.go` — `ExpressionClass` (the class names of `ChainType`'s result, no type arguments), `Engine.DiagnosticType` and `NewChainEngine`, which resolves chains in assignments with `ExpressionClass`: what diagnostics type expressions with, without a completion provider.
- `assignment.go` — the value an assignment gives a variable, read from its node.
- `closure_type.go`, `closure_context.go` — closures as `Closure(…): T`, and the call a closure is passed to (its parameter types, `@param-closure-this`).
- `mixins.go` — generic `@mixin T`; `narrowing.go` — `@phpstan-assert*`, `@param-out`, `@phpstan-self-out`, and the `instanceof` tests of guards, enclosing `if`/`elseif`/`else`/`while` blocks, `switch (true)` cases and the read's own `&&`/`||`/`?`/`match (true)`, all read from the syntax tree.
- `parse_cache.go`, `source_cache.go` — resolver-owned caches: parsed docblocks and PHPDoc types by text, and the last file's source, lines and scopes. Cached results are read-only.

Templates are stored at index time (`symbols.Symbol.Templates` on classes, methods and functions; `Supertypes` for `@extends` and friends). PHPDoc types read from another class must go through `applyEnv` with that class as `owner`, so names relative to its namespace resolve there. `internal/lsp/generics_test.go` checks every generics feature through the running server.

## What lives here vs elsewhere

- **Here:** expression → type, type → type (substitution), PHPDoc → structured type, generic mappings, enclosing-scope lookups, FQN building, line/word helpers used by resolution.
- **Not here:** raw lexing/AST (that's `parser/`), the symbol store itself (that's `symbols/`), cursor-position classification beyond enclosing scope (that's `source/`), variable-scope tracking (that's `scope/`).

## Conventions

- Prefer `ResolvedType` over passing raw type strings around. The string form is fine for hover output, but internal flow should be structured.
- `PickBestUnionPart` and `HasNullInUnion` are intentionally exported — they're used by completion/hover and tested via `*_coverage_test.go`. Don't inline-copy them.
- Recursion guards exist (`recursion_test.go`) because chain resolution and template expansion can loop on self-referential generics. Preserve them. The guard lives on the request's `Resolver`, so it only holds while one `Resolver` is passed through the whole request.
- New PHPDoc handling belongs in `internal/types` (the structured type) and here (reading it into `ResolvedType`), not in feature-specific string parsing.

The project-wide rules in the root [AGENTS.md](../../AGENTS.md) apply here too.
