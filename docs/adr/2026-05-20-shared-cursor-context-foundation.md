# ADR: Shared Cursor Context Foundation

## Status

Accepted

## Context

`ROADMAP.md` Sprint 1 prioritizes a shared cursor-context abstraction for
completion, hover, definition, references, and rename. Before this change,
those packages each rebuilt overlapping state from raw source:

- namespace and `use` imports
- word/range extraction at the cursor
- member-access detection (`->`, `?->`, `::`)
- multi-line chain joining
- enclosing class and method context

That duplication had already started to drift. The same cursor could be
classified slightly differently depending on which provider handled it.

## Decision

Add a new internal package at `internal/source` with a small first-pass API:

- `Analyze(uri, source, pos)` returns a `CursorContext`
- `WordAt(source, pos)` exposes exact identifier/range extraction
- `Namespace(source)` centralizes namespace lookup
- `DetectMemberContext(trimmed)` centralizes typed-member prefix detection for completion

The initial `CursorContext` carries:

- namespace and active imports
- symbol text and exact range
- symbol kind (`identifier` or `variable` for now)
- access kind (`none`, `instance`, `nullsafe`, `static`)
- subject expression before the access operator
- enclosing class FQN
- enclosing method scope metadata
- parsed file handle
- joined chain line and derived member start offset

## Implementation

Implemented:

- new package: `internal/source`
- focused tests for namespace, word/range extraction, member-context detection,
  and multi-line chain context
- completion now uses shared namespace/context parsing and shared typed-member
  detection
- hover now uses shared cursor analysis for symbol text, enclosing class,
  imports, and joined-chain metadata
- analyzer now uses shared word/range and access-context analysis in
  definition/references/rename paths

## Tradeoffs

- This is intentionally not a full semantic source model. It consolidates the
  duplicated mechanics that already existed and leaves deeper context work for
  later milestones.
- `Analyze` parses the file per provider request. That keeps the integration
  scoped and low-risk now, but it is not yet a shared parse/cache layer.
- `Scope` is minimal. It currently models enclosing class/method boundaries,
  not full variable scope collection.

## Consequences

- Cursor-sensitive features now share one implementation for the most common
  position-derived facts.
- Future work can extend `CursorContext` instead of adding more provider-local
  heuristics.
- The old completion-specific member detector was removed to avoid reintroducing
  divergence.

## Remaining Work

- extend symbol classification beyond identifier/variable into imports, string
  literals, PHPDoc, attributes, and array-key contexts
- move more provider-local context parsing into `internal/source`
- decide whether parse results should be cached per document request cycle
- connect the later roadmap `scope` foundation to this package instead of
  embedding richer scope logic here prematurely
