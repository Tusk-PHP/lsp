<p align="center">
  <img src="art/tusk-php-icon.png" alt="Tusk PHP logo" width="180">
</p>

<h1 align="center">Tusk PHP</h1>

<p align="center">
  <a href="https://github.com/Tusk-PHP/lsp/actions/workflows/test.yml"><img alt="CI" src="https://github.com/Tusk-PHP/lsp/actions/workflows/test.yml/badge.svg"></a>
  <a href="https://github.com/Tusk-PHP/lsp/actions/workflows/release.yml"><img alt="Release" src="https://github.com/Tusk-PHP/lsp/actions/workflows/release.yml/badge.svg"></a>
  <a href="https://codecov.io/gh/Tusk-PHP/lsp"><img alt="codecov" src="https://codecov.io/gh/Tusk-PHP/lsp/graph/badge.svg?token=BgBvrfITKq"></a>
  <a href="https://github.com/Tusk-PHP/lsp/releases/latest"><img alt="GitHub Release" src="https://img.shields.io/github/v/release/Tusk-PHP/lsp"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-blue.svg"></a>
  <img alt="PHP 8.0-8.5" src="https://img.shields.io/badge/PHP-8.0--8.5-777BB4?logo=php&logoColor=white">
</p>

<p align="center">
  A high-performance PHP language server written in Go, with framework-aware Laravel and Symfony intelligence.
</p>

> [!NOTE]
> Tusk PHP is in active development. Completion, navigation, refactoring, and native diagnostics are used daily on Laravel and Symfony codebases. Some resolution paths — noted in the [correctness benchmarks](#correctness) below — are still catching up to more mature engines; closing them is ongoing work in the shared [`resolve/`](internal/resolve) type-resolution layer, not a fundamental limitation.

## Benchmarks

Measured with the repository's own [`bench/`](bench/) harness: every server gets the identical LSP requests at the identical caret positions in the identical workspaces, over stdio, with a cold cache and one server running at a time. See [`bench/README.md`](bench/README.md) for the full methodology, including how correctness is graded and why "ready" — not `initialize` — is the only start-up number comparable across servers.

> [!TIP]
> **Want to verify?** `bash bench/tools/install.sh && go run ./bench/lspbench` fetches the other servers and runs the whole suite, writing `results.json` and `report.md` to `bench/results/<timestamp>/`. Add `-v` to see the request/response for any single case, or `-servers tusk-php -workspaces laravel` to scope a run.

### Startup and memory

Time to `ready` (first correct cross-file answer) and peak RSS, per workspace. Measured on an Apple M3 Max, PHP 8.5.10, cold cache.

| workspace | Tusk PHP | Intelephense | Phpactor | php-lsp (Rust) | PHPantom (Rust) |
| --- | --- | --- | --- | --- | --- |
| `diagnostics` (6 files) | 29 ms / 25 MB | 199 ms / 293 MB | 3325 ms / 159 MB | 155 ms / 118 MB | 114 ms / 32 MB |
| `symfony` (1.1k files) | 285 ms / 54 MB | 695 ms / 362 MB | 3530 ms / 111 MB | 207 ms / 171 MB | 438 ms / 56 MB |
| `laravel` (9.7k files) | 283 ms / 162 MB | 2226 ms / 686 MB | 4391 ms / 198 MB | 260 ms / 272 MB | 847 ms / 88 MB |

<sub>Format: ready time / peak RSS. Tusk PHP and PHPantom are the two lowest-memory servers across every workspace; php-lsp is occasionally faster to ready on the larger ones, and every Go/Rust server beats the Node (Intelephense) and PHP (Phpactor) runtimes by a wide margin on both counts.</sub>

### Correctness

Cases passed / cases graded, per workspace, across completion, definition, hover, references, signature help, document/workspace symbols, and diagnostics. A case only counts against a server if that server advertises the capability; `err` means it advertised support and then failed to answer before the timeout.

| workspace | Tusk PHP | Intelephense | Phpactor | php-lsp (Rust) | PHPantom (Rust) |
| --- | --- | --- | --- | --- | --- |
| `diagnostics` | 4/6 | 6/6 | 5/6 | 6/6 | 6/6 |
| `laravel` | 11/19 | 12/19 | 14/19 | 7/15 (+4 err) | 16/19 |
| `symfony` | 7/11 | 11/11 | 11/11 | 6/8 (+3 err) | 9/11 |

<sub>Full per-case breakdowns are written to <code>bench/results/&lt;timestamp&gt;/report.md</code> after a run. Tusk PHP's current gaps cluster around Eloquent/query-builder hover resolution and a handful of completion edge cases — see the note above.</sub>

## Intelligent Completions

Context-aware suggestions that understand your code. Method chains resolve through return types and `@return` docblocks, so `Category::query()->with('tags')->where()->` shows the right `Builder` methods, not random matches from vendor.

- **Member access** (`->`, `::`, `?->`) with full chain resolution
- **Auto-import** — selecting a class automatically adds the `use` statement
- **Smart parens** — skips adding `()` when they already exist
- **Container bindings** — `app('request')->` completes with `Request` methods
- **Config keys** — `config('database.')` navigates with dot-notation
- **Array shapes** — `$config['']` completes keys from `@var` annotations and literal arrays
- **`new`, `use`, type hints, attributes, pipe operator `|>`**

## Hover & Navigation

- **Hover** — type signatures, docblock summaries, inheritance info, container bindings
- **PHP Manual Links** — hover on built-in functions, classes, methods, properties, and constants includes a link to the php.net manual page. Optionally configure `php_manual_open_on_definition` to make Cmd+Click open the page in your browser.
- **Go to Definition** — jump to classes, methods, properties, functions, container bindings, framework string references
- **Go to Type Definition** — resolve container services and member types to concrete declarations
- **Go to Implementation** — find implementations for interfaces and abstract contracts
- **Find References** — all usages across the workspace, including member access chains
- **Document Symbols** — full file outline with classes, methods, properties, constants
- **Workspace Symbols** — search indexed classes, members, functions, and constants across the project
- **Document Highlights** — highlight declaration, read, and write occurrences in the current file
- **Folding Ranges** — fold namespaces, declarations, arrays, comments, and nested regions
- **Signature Help** — parameter hints while typing function calls
- **Rename** — rename variables (scoped), classes, methods, properties across the workspace

## Inlay Hints

Inline hints are enabled by default for:

- inferred local variable types
- `foreach` key and value types
- closure return types
- method return types from PHPDoc
- call-site parameter names

## Refactoring

- **Copy Namespace** — copy the fully qualified name of the current file's class
- **Move to Namespace** — move a class to a different namespace, updating all references and the file path per PSR-4
- **Import Code Actions** — remove unused imports, organize imports, and import unknown classes
- **Generate Code** — implement missing methods, generate constructors, and generate safe getters/setters
- **Local Refactors** — extract a selected expression to a variable and inline safe single-assignment variables

## Diagnostics

Built-in static analysis rules run on every change (fast checks) or on save (heavier checks). All rules are individually configurable.

| Rule | Code | Severity | Runs on |
|------|------|----------|---------|
| Unused imports | `unused-import` | Hint | Change |
| Unused private methods | `unused-private-method` | Info | Change |
| Unused private properties | `unused-private-property` | Hint | Change |
| Unreachable code | `unreachable-code` | Warning | Change |
| Redundant union members | `redundant-union-member` | Info | Change |
| Redundant nullsafe `?->` | `redundant-nullsafe` | Info | Save |
| Unknown column in Builder | `unknown-column` | Warning | Save |
| Unknown relation in Builder | `unknown-relation` | Warning | Save |
| Unknown class | `unknown-class` | Warning | Change |
| Unknown function | `unknown-function` | Warning | Change |
| Unknown member | `unknown-member` | Warning | Change |
| Deprecated PHP functions | `deprecated` | Warning | Change |
| Abstract method in concrete class | `abstract-in-concrete` | Error | Change |

Unused imports and unused private members are tagged as `Unnecessary` (editors grey them out). Deprecated functions are tagged as `Deprecated` (editors strike them through).

External tool integrations:

- **PHPStan** — runs on save, shows errors inline (requires PHPStan in your project)
- **Laravel Pint** — formatting diagnostics on save (requires Pint in your project)

## Framework Intelligence

### Laravel

Scans service providers for `bind()` / `singleton()` calls and pre-loads 25+ core framework bindings. Completions and go-to-definition work through:

- `app('request')`, `resolve(Cache::class)`, `$this->app->make(...)`
- `config()` with dot-notation key navigation and value preview
- Eloquent method chains: `Model::query()->with()->where()->get()`
- route names, view names, and translation keys with completion and go-to-definition
- Interface-to-concrete resolution from container bindings

### Symfony

Parses `services.yaml`, PHP service configs, route config, and autowiring attributes:

- `#[Autowire]`, `#[AsController]`, `#[AsCommand]`, `#[AsEventListener]`, `#[AsMessageHandler]`
- Auto-wired services from `src/`
- `$container->get(...)` with registered service completions and service/type definition navigation
- route names from controller attributes and `config/routes.{yaml,yml,xml}` imports
- Interface-to-concrete resolution from `implements` declarations

### PHP 8.0 - 8.5

Full support for modern PHP syntax: union/intersection/DNF types, enums, fibers, readonly classes, match expressions, named arguments, attributes, property hooks, asymmetric visibility, typed constants, pipe operator `|>`.

## Installation

### VS Code / VS Codium

Install the extension from the registry used by your editor:

- [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=open-southeners.tusk-php) for Visual Studio Code.
- [Open VSX Registry](https://open-vsx.org/extension/open-southeners/tusk-php) for VS Codium and other Open VSX-based editors.

VS Code can also install it from the command palette or CLI:

```
ext install open-southeners.tusk-php
```

The extension bundles the language server binary — no additional setup required.

### Zed

Search for **"Tusk PHP"** in `zed: extensions`. The extension automatically downloads the correct binary for your platform.

Keep Zed's built-in **PHP** extension installed. It provides syntax highlighting, PHPDoc highlighting, and other language assets. Install **Tusk PHP** alongside it, then set Tusk PHP as the only PHP language server to avoid collisions with Phpactor, Intelephense, or PHP Tools:

```json
{
  "languages": {
    "PHP": {
      "language_servers": ["tusk-php"]
    }
  }
}
```

Use Zed's `lsp: restart language servers` action after changing settings or reinstalling the dev extension.

Zed-specific Tusk PHP settings can be passed through `lsp.tusk-php`:

```json
{
  "lsp": {
    "tusk-php": {
      "binary": {
        "path": "/absolute/path/to/tusk-php",
        "arguments": ["--transport", "stdio"]
      },
      "initialization_options": {
        "phpVersion": "8.5",
        "framework": "auto",
        "containerAware": true,
        "diagnosticsEnabled": true,
        "phpstanEnabled": true,
        "phpstanPath": "",
        "phpstanLevel": "",
        "phpstanConfig": "",
        "pintEnabled": true,
        "pintPath": "",
        "pintConfig": "",
        "maxIndexFiles": 10000,
        "excludePaths": ["vendor", "node_modules", ".git", "storage", "var/cache"]
      }
    }
  }
}
```

For local language-server development, build the binary with `make build` and
set `lsp.tusk-php.binary.path` to the absolute path of `build/tusk-php`; Zed
will use that binary instead of downloading the release artifact. See
[CONTRIBUTING.md](CONTRIBUTING.md#testing-with-zed) for the full local Zed
testing workflow.

The Zed extension also provides Assistant slash commands:

```text
/tusk-copy-namespace app/Models/User.php
/tusk-namespace-for-path app/Models/User.php
```

Zed does not expose the same extension command API as VS Code, so `Restart Server` maps to Zed's built-in `lsp: restart language servers` action, and `Move to Namespace...` is not yet exposed as a Zed command.

### Neovim

Install the binary (see below), then add to your config:

```lua
require('lspconfig.configs').php_lsp = {
  default_config = {
    cmd = { 'tusk-php', '--transport', 'stdio' },
    filetypes = { 'php' },
    root_dir = require('lspconfig.util').root_pattern('composer.json', '.git'),
  }
}
require('lspconfig').php_lsp.setup({})
```

### Other Editors

Any LSP client works. The server uses **stdio** with JSON-RPC 2.0:

```bash
tusk-php --transport stdio
```

### Binary Installation

**Download** from [GitHub Releases](https://github.com/Tusk-PHP/lsp/releases/latest) (Linux, macOS, Windows — amd64/arm64).

**Quick install** (Linux / macOS):

```bash
curl -fsSL https://raw.githubusercontent.com/Tusk-PHP/lsp/main/scripts/install.sh | bash
```

**From source** (requires Go 1.22+):

```bash
git clone https://github.com/Tusk-PHP/lsp.git tusk-php && cd tusk-php && make install
```

## AI Agents (MCP)

`tusk-php mcp` exposes the project's indexed intelligence — symbols, native diagnostics, container bindings, live database schema, and framework routes/models — to AI coding agents over the [Model Context Protocol](https://modelcontextprotocol.io). It is a subcommand of the same `tusk-php` binary the editor already uses; there is no separate binary or extra download.

There's nothing to launch manually. The agent spawns `tusk-php mcp` itself over **stdio** using the command and arguments in your config, and shuts it down when it disconnects. It indexes the project at `--root` (defaults to the current working directory).

### Claude Code

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

### Codex

Add the server to `~/.codex/config.toml`:

```toml
[mcp_servers.tusk-php]
command = "tusk-php"
args = ["mcp", "--root", "/absolute/path/to/your/php-project"]
```

### Other Agents (Cursor, Windsurf, generic MCP clients)

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

### What the agent gets

Framework-specific tools appear only when that framework is detected:

| Tool | Purpose |
|------|---------|
| `php_project_summary` | Project metadata: framework, indexed file count, live-schema availability |
| `php_find_symbol` | Find symbols by exact name, name prefix, or FQN prefix |
| `php_explain_symbol` | Explain a symbol by FQN — members, inheritance, interfaces |
| `php_find_references` | Find references to a symbol by FQN |
| `php_diagnostics` | Native in-process diagnostics for one file or the whole project (excludes PHPStan/Pint) |
| `php_container_bindings` | Resolved DI bindings (interface→concrete, singletons, tags); optionally a class's constructor injection |
| `db_list_tables`, `db_describe_table`, `db_find_column` | Query the live database schema snapshot |
| `laravel_routes`, `laravel_route_to_controller`, `laravel_model_schema` | Laravel route and Eloquent model intelligence |
| `symfony_routes`, `symfony_route_to_controller` | Symfony route intelligence |
| `tusk_reindex` | Rebuild the workspace snapshot on demand |

The server also exposes the same data as readable JSON resources: `project-summary`, `php-symbols`, and `db-schema-compact`, plus `laravel-routes` / `laravel-models` or `symfony-routes` depending on the detected framework.

## Configuration

### Editor Settings (VS Code)

| Setting | Default | Description |
|---------|---------|-------------|
| `tuskPhpLsp.enable` | `true` | Enable/disable the extension |
| `tuskPhpLsp.executablePath` | `""` | Custom path to the binary |
| `tuskPhpLsp.phpVersion` | `"8.5"` | Target PHP version (`8.0`-`8.5`) |
| `tuskPhpLsp.framework` | `"auto"` | Framework: `auto`, `laravel`, `symfony`, `none` |
| `tuskPhpLsp.containerAware` | `true` | Enable DI container analysis |
| `tuskPhpLsp.diagnostics.enable` | `true` | Enable diagnostics |
| `tuskPhpLsp.diagnostics.phpstan.enable` | `true` | Run PHPStan on save |
| `tuskPhpLsp.diagnostics.pint.enable` | `true` | Run Laravel Pint on save |
| `tuskPhpLsp.maxIndexFiles` | `10000` | Maximum PHP files to index |
| `tuskPhpLsp.excludePaths` | `["vendor", ...]` | Paths to skip when indexing |
| `tuskPhpLsp.phpManual.locale` | `""` | Locale for php.net manual links (`en`, `de`, `es`, `fr`, `it`, `ja`, `pt_BR`, `ru`, `tr`, `zh`). Empty = English. |
| `tuskPhpLsp.phpManual.openOnDefinition` | `false` | Cmd+Click on a built-in opens its php.net page in your browser. |

### Project Configuration

Create `.tusk-php.json` in your project root to override settings per project:

```json
{
  "phpVersion": "8.5",
  "framework": "auto",
  "containerAware": true,
  "diagnosticsEnabled": true,
  "excludePaths": ["vendor", "node_modules", ".git"],
  "maxIndexFiles": 10000,
  "php_manual_locale": "",
  "php_manual_open_on_definition": false,
  "diagnosticRules": {
    "unused-import": true,
    "unused-private-method": true,
    "unused-private-property": true,
    "unreachable-code": true,
    "redundant-nullsafe": true,
    "redundant-union-member": true,
    "unknown-column": true,
    "unknown-relation": true,
    "unknown-class": true,
    "unknown-function": true,
    "unknown-member": true
  },
  "inlayHints": {
    "enabled": true,
    "variableTypes": true,
    "foreachTypes": true,
    "closureReturnTypes": true,
    "returnTypes": true,
    "parameterNames": true
  }
}
```

All diagnostic rules are enabled by default. Set a rule to `false` to disable it. Framework is auto-detected from `artisan` (Laravel), `bin/console` (Symfony), or `composer.json`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

[MIT](LICENSE)
