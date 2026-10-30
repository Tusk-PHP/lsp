# AI agents (MCP)

Back to the [README](../README.md).

`tusk-php mcp` exposes the project's indexed intelligence — symbols, native diagnostics, container bindings, live database schema, and framework routes/models — to AI coding agents over the [Model Context Protocol](https://modelcontextprotocol.io). It is a subcommand of the same `tusk-php` binary the editor already uses; there is no separate binary or extra download.

There's nothing to launch manually. The agent spawns `tusk-php mcp` itself over **stdio** using the command and arguments in your config, and shuts it down when it disconnects. It indexes the project at `--root` (defaults to the current working directory).

## Claude Code

Register the server for the current project (writes a project-scoped `.mcp.json`):

```bash
claude mcp add tusk-php --scope project -- tusk-php mcp --root "$(pwd)"
```

Or add it by hand to `.mcp.json` in the project root:

```json
{
  "mcpServers": {
    "tusk-php": {
      "command": "tusk-php",
      "args": ["mcp", "--root", "."]
    }
  }
}
```

Run `/mcp` inside Claude Code to confirm the `tusk-php` server is connected and its tools are listed.

## Codex

Add the server to `~/.codex/config.toml`:

```toml
[mcp_servers.tusk-php]
command = "tusk-php"
args = ["mcp", "--root", "/absolute/path/to/your/php-project"]
```

## Other Agents (Cursor, Windsurf, generic MCP clients)

Any client that launches stdio MCP servers uses the same shape — a command plus arguments:

```json
{
  "mcpServers": {
    "tusk-php": {
      "command": "tusk-php",
      "args": ["mcp", "--root", "/absolute/path/to/your/php-project"]
    }
  }
}
```

If the binary is not on your `PATH`, use its absolute path as `command`.

## What the agent gets

Framework-specific tools appear only when that framework is detected:

| Tool | Purpose |
|------|---------|
| `php_project_summary` | Project metadata: framework, indexed file count, live-schema availability |
| `php_find_symbol` | Find symbols by exact name, name prefix, or FQN prefix |
| `php_explain_symbol` | Explain a symbol by FQN — members, inheritance, interfaces |
| `php_find_references` | Find references to a symbol by FQN |
| `php_diagnostics` | Native in-process diagnostics for one file or the whole project (excludes PHPStan/Pint) |
| `php_coverage` | Test coverage from your last Clover report: a file's uncovered lines and untested methods, or project totals and the least covered files. Doesn't run tests. |
| `php_container_bindings` | Resolved DI bindings (interface→concrete, singletons, tags); optionally a class's constructor injection |
| `db_list_tables`, `db_describe_table`, `db_find_column` | Query the live database schema snapshot |
| `laravel_routes`, `laravel_route_to_controller`, `laravel_model_schema` | Laravel route and Eloquent model intelligence |
| `symfony_routes`, `symfony_route_to_controller` | Symfony route intelligence |
| `tusk_reindex` | Rebuild the workspace snapshot on demand |

The server also exposes the same data as readable JSON resources: `project-summary`, `php-symbols`, and `db-schema-compact`, plus `laravel-routes` / `laravel-models` or `symfony-routes` depending on the detected framework.
