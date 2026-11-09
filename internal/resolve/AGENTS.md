# internal/resolve

Shared type and symbol resolution. Hover, completion, diagnostics, and the analyzer should all agree on types by going through this package.

## Entry points

- `Resolver` (`resolve.go`) — owns a `*symbols.Index` and resolves expressions/types in context.
- `ResolvedType` (`types.go`) — `{FQN, Params[]}` plus the structured parts: `Union`, `Intersection`, `Callable`, shape `Fields`, `Literal` and `Conditional`. For a union, `FQN`/`Params` mirror its most useful part, so single-class readers keep working; `FullString()` shows all of it, `MemberOwner()` gives every class to look members up on. The canonical type representation everywhere downstream.
- `ParseGenericType(raw)` / `InferArrayLiteralType(literal)` — produce `ResolvedType` from raw strings.
- `ResolveTemplateReturn(classFQN, methodName, typeArgs)` (`templates.go`) — applies `@template` substitution for known generic methods (e.g. `Collection<T>::first(): T`).
- `FindEnclosingClass(file, pos)` / `FindEnclosingMethod(file, pos)` — scope helpers used by every provider.
- `doc_types.go` — PHPDoc type parsing (unions, nullable, generics, array shapes).
- `type_aliases.go` — `@phpstan-type` / `@phpstan-import-type` aliases.

### Generics engine (PHPStan semantics, issue #68)

- `generics.go` — `applyEnv` (substitute templates and `static`, resolve names, evaluate `key-of`/`value-of`/`T[K]`/`new<T>`), `bindTemplates` (match a parameter's declared type against an argument's type), `AncestorTypeArgs` (`@extends`/`@implements`/`@use` arguments through the hierarchy), `fillUnbound` (default, then bound, then `mixed`), `IterableTypes`.
- `call.go` — `MemberTypeForCall` / `FunctionTypeForCall`: what a member or function gives at a call, with the receiver's and the call's templates bound and conditional return types decided. `MemberTypeResolved` is the same without arguments.
- `member_types.go` — `rawMemberReturnType` / `preferDocType`: when PHPDoc wins over the native type.
- `expression.go` — `ExpressionType`: the type of an argument or right-hand side (variables, literals, `new`, `::class`, arrays, closures, function calls, chains, offsets).
- `closure_type.go`, `closure_context.go` — closures as `Closure(…): T`, and the call a closure is passed to (its parameter types, `@param-closure-this`).
- `mixins.go` — generic `@mixin T`; `narrowing.go` — `@phpstan-assert*`, `@param-out`, `@phpstan-self-out`.
- `parse_cache.go`, `source_cache.go` — resolver-owned caches: parsed docblocks and PHPDoc types by text, and the last file's source, lines and scopes. Cached results are read-only.

Templates are stored at index time (`symbols.Symbol.Templates` on classes, methods and functions; `Supertypes` for `@extends` and friends). PHPDoc types read from another class must go through `applyEnv` with that class as `owner`, so names relative to its namespace resolve there. `internal/lsp/generics_test.go` checks every generics feature through the running server.

## What lives here vs elsewhere

- **Here:** expression → type, type → type (substitution), PHPDoc → structured type, generic mappings, enclosing-scope lookups, FQN building, line/word helpers used by resolution.
- **Not here:** raw lexing/AST (that's `parser/`), the symbol store itself (that's `symbols/`), cursor-position classification beyond enclosing scope (that's `source/`), variable-scope tracking (that's `scope/`).

## Conventions

- Prefer `ResolvedType` over passing raw type strings around. The string form is fine for hover output, but internal flow should be structured.
- `PickBestUnionPart`, `HasNullInUnion`, `IndexOutsideBrackets` are intentionally exported — they're used by completion/hover and tested via `*_coverage_test.go`. Don't inline-copy them.
- Recursion guards exist (`recursion_test.go`) because chain resolution and template expansion can loop on self-referential generics. Preserve them.
- The roadmap direction is to widen `doc_types.go` into a fuller structured PHPDoc model (intersections, conditional return types, callable signatures). New PHPDoc handling belongs here, not in feature-specific string parsing.

The project-wide rules in the root [AGENTS.md](../../AGENTS.md) apply here too.
