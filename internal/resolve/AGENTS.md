# internal/resolve

Shared type and symbol resolution. Hover, completion, diagnostics, and the analyzer should all agree on types by going through this package.

## Entry points

- `Resolver` (`resolve.go`) — owns a `*symbols.Index` and resolves expressions/types in context.
- `ResolvedType` (`types.go`) — `{FQN, TypeArgs[]}`. The canonical type representation everywhere downstream.
- `ParseGenericType(raw)` / `InferArrayLiteralType(literal)` — produce `ResolvedType` from raw strings.
- `ResolveTemplateReturn(classFQN, methodName, typeArgs)` (`templates.go`) — applies `@template` substitution for known generic methods (e.g. `Collection<T>::first(): T`).
- `FindEnclosingClass(file, pos)` / `FindEnclosingMethod(file, pos)` — scope helpers used by every provider.
- `doc_types.go` — PHPDoc type parsing (unions, nullable, generics, array shapes).
- `type_aliases.go` — `@phpstan-type` / `@phpstan-import-type` aliases.

## What lives here vs elsewhere

- **Here:** expression → type, type → type (substitution), PHPDoc → structured type, generic mappings, enclosing-scope lookups, FQN building, line/word helpers used by resolution.
- **Not here:** raw lexing/AST (that's `parser/`), the symbol store itself (that's `symbols/`), cursor-position classification beyond enclosing scope (that's `source/`), variable-scope tracking (that's `scope/`).

## Conventions

- Prefer `ResolvedType` over passing raw type strings around. The string form is fine for hover output, but internal flow should be structured.
- `PickBestUnionPart`, `HasNullInUnion`, `IndexOutsideBrackets` are intentionally exported — they're used by completion/hover and tested via `*_coverage_test.go`. Don't inline-copy them.
- Recursion guards exist (`recursion_test.go`) because chain resolution and template expansion can loop on self-referential generics. Preserve them.
- The roadmap direction is to widen `doc_types.go` into a fuller structured PHPDoc model (intersections, conditional return types, callable signatures). New PHPDoc handling belongs here, not in feature-specific string parsing.

The project-wide rules in the root [AGENTS.md](../../AGENTS.md) apply here too.
