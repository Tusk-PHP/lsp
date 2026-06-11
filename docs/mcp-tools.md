# Tusk MCP Tools and Resources Reference

This document describes every tool and resource exposed by the `tusk-mcp` MCP server.

## Contract Version

The `php_project_summary` tool includes a `contractVersion` field. The current value is **1**.

Consumers (AI agents, IDE plugins) should check `contractVersion` at startup to detect breaking changes.
The dump context-pack format (produced by `tusk mcp dump`) uses the same versioning.
Graph DTOs carry `schemaVersion` (currently **1**) in every graph payload.

---

## Universal Tools

These tools are available regardless of detected framework.

### `php_project_summary`

Return a concise summary of the PHP project.

**Input:** none

**Output:**

| Field | Type | Description |
|-------|------|-------------|
| `rootPath` | string | Absolute workspace root |
| `framework` | string | Detected framework: `laravel`, `symfony`, `none` |
| `phpVersion` | string | Effective PHP version (e.g. `8.2`) |
| `phpVersionSource` | string | How the version was detected |
| `indexedFiles` | int | Number of indexed PHP files |
| `routeCount` | int | Number of discovered routes |
| `schemaSource` | string | DB schema source (`live`, `migrations`, `none`) |
| `schemaTables` | []string | Sorted table names (omitted when empty) |
| `contractVersion` | int | MCP tool contract version (**currently 1**) |

**Gating:** always available

---

### `php_find_symbol`

Find PHP symbols by exact name, name prefix, or FQN prefix.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | yes | Symbol name or FQN prefix |
| `limit` | int | no | Max results (default 20) |

**Output:**

```json
{ "query": "...", "results": [ { "name": "...", "fqn": "...", "kind": "...", "uri": "...", "source": "..." } ] }
```

**Gating:** always available

---

### `php_explain_symbol`

Explain a PHP symbol by FQN, including members, inheritance, and interfaces where applicable.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fqn` | string | yes | Fully qualified symbol name |

**Output:** `SymbolExplanation` with `symbol`, `type`, `returnType`, `docComment`, `inheritanceChain`, `implementedInterfaces`, `memberCount`, `members`, `descendants`, `implementors`, `namespaceMembers`.

**Gating:** always available

---

### `php_find_references`

Find references to a PHP symbol by FQN.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fqn` | string | yes | Fully qualified symbol name |

**Output:** `{ "fqn": "...", "references": [ { "uri": "...", "range": { ... } } ] }`

**Gating:** always available. Only references in AI-allowed paths are returned.

---

### `php_diagnostics`

Run native in-process diagnostics for one PHP file or the full indexed project.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | no | Workspace-relative or absolute PHP file path. Omit to scan all files. |

**Output:** `{ "files": [ { "path": "...", "diagnostics": [ ... ] } ] }`

External tools (PHPStan, Pint) are not run.

**Gating:** always available. Paths outside `ai.allowedRoots` or inside `ai.denyPaths` are rejected.

---

### `php_graph`

Build and return a PHP dependency graph.

**Input:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `kind` | string | yes | — | `container`, `references`, or `models` |
| `deps` | string | no | `none` | Dependency mode: `none` (omit vendor/builtin), `boundary` (collapse vendor to package boundaries), `full` (include all nodes) |
| `format` | string | no | `json` | `json`, `mermaid`, or `dot` |

**Output (json):** Graph DTO `{ "schemaVersion": 1, "kind": "...", "nodes": [...], "edges": [...] }`.

- `schemaVersion` is always **1** (see `internal/graph.SchemaVersion`)
- Nodes have `{ "id", "kind", "label", "meta?" }`. Node kinds: `class`, `binding`, `dependency-boundary`, `model`
- Edges have `{ "from", "to", "kind" }`. Edge kinds: `binds`, `injects`, `extends`, `implements`, `uses`, `property`, `param`, `returns`, `new`, plus relation types for models (e.g. `HasMany`, `BelongsTo`)

**Output (mermaid/dot):** `{ "format": "mermaid"|"dot", "text": "..." }` containing the rendered diagram text.

**Gating:** always available. The graph is built on demand against the current workspace snapshot; reindex invalidates the working data.

---

### `tusk_reindex`

Rebuild the MCP workspace snapshot and atomically swap it into service.

**Input:** none

**Output:**

| Field | Type | Description |
|-------|------|-------------|
| `rootPath` | string | Workspace root |
| `framework` | string | Detected framework |
| `indexedFiles` | int | Count after reindex |
| `routeCount` | int | Route count after reindex |
| `schemaSource` | string | DB schema source |
| `schemaTables` | []string | Table names |
| `phpVersion` | string | Effective PHP version |
| `phpVersionSource` | string | Version source |

**Gating:** always available

---

## Database Tools

### `db_list_tables`

List tables from the normalized live database schema snapshot.

**Input:** none

**Output:** `{ "connection": "...", "database": "...", "source": "...", "connected": bool, "caveat": "...", "tables": [...] }`

**Gating:** always available (returns empty list when no schema is configured)

---

### `db_describe_table`

Describe a live database table using the normalized schema snapshot.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Database table name |

**Output:** `{ "connection", "database", "source", "connected", "caveat", "table": { ... } }`

**Gating:** always available. Returns an error when the schema is unavailable.

---

### `db_find_column`

Find a database column across all known tables in the normalized schema snapshot.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Column name to search |

**Output:** `{ "connection", "database", "source", "connected", "caveat", "matches": [ { "table": "...", "column": { ... } } ] }`

**Gating:** always available

---

## Laravel Tools

These tools and resources are only registered when the workspace is detected as a Laravel project.

### `laravel_routes` (tool)

List known Laravel route names and their definition locations.

**Input:** none

**Output:** `{ "routes": [ { "name", "path", "uri", "range", "targetKind", "target", "handlerFqn", "handlerMethod", "handlerDefinition"?, "handlerRange"? } ] }`

**Gating:** always registered (returns empty list for non-Laravel workspaces; use `laravel-routes` resource for framework-gated access)

---

### `laravel_route_to_controller`

Best-effort Laravel route definition lookup by route name.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Laravel route name |

**Output:** Single `LaravelRouteRecord` (same fields as above). Returns an error if the route is not found.

**Gating:** always registered

---

### `laravel_model_schema`

Describe a Laravel Eloquent model, its resolved table, and known members.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fqn` | string | yes | Fully qualified Laravel model class |

**Output:** `{ "model"?, "tableName", "table"?, "members", "memberCount" }`

**Gating:** always registered

---

## Symfony Tools

These tools and resources are only registered when the workspace is detected as a Symfony project.

### `symfony_routes` (tool)

List known Symfony route names, paths, and declaration file locations.

**Input:** none

**Output:** `{ "routes": [ { "name", "path", "uri", "declRange": { "start": {...}, "end": {...} } } ] }`

Sources: PHP attribute routes (`#[Route(...)]`) and YAML/XML route config files.

**Gating:** Symfony workspace only

---

### `symfony_route_to_controller`

Look up a Symfony route by name and return its declaration location.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Symfony route name |

**Output:** Single `SymfonyRouteRecord` (same fields as above). Returns an error if the route is not found.

**Gating:** Symfony workspace only

---

## Resources

Resources are served at stable `tusk://` URIs and contain JSON.

| URI | MIME | Description | Gating |
|-----|------|-------------|--------|
| `tusk://project/summary` | `application/json` | `ProjectSummary` (same as `php_project_summary`) | always |
| `tusk://db/schema/compact` | `application/json` | Compact schema: `{ "source", "connection", "database", "connected", "caveat", "tables": { "tableName": { "column": "type" } } }` | always |
| `tusk://php/symbols` | `application/json` | Compact top-level symbol catalog (class/interface/trait/enum/function/namespace entries) | always |
| `tusk://laravel/routes` | `application/json` | Laravel route array (same shape as `laravel_routes` output) | Laravel only |
| `tusk://laravel/models` | `application/json` | Array of `{ symbol, tableName }` for all Eloquent models | Laravel only |
| `tusk://symfony/routes` | `application/json` | Symfony route array (same shape as `symfony_routes` output) | Symfony only |
