# Editor setup

How to install and configure Tusk PHP in each editor. Back to the [README](../README.md).

## VS Code / VS Codium

Install the extension from the registry used by your editor:

- [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=open-southeners.tusk-php) for Visual Studio Code.
- [Open VSX Registry](https://open-vsx.org/extension/open-southeners/tusk-php) for VS Codium and other Open VSX-based editors.

VS Code can also install it from the command palette or CLI:

```
ext install open-southeners.tusk-php
```

The extension bundles the language server binary — no additional setup required.

## Zed

> [!NOTE]
> The Zed extension isn't in Zed's extension registry yet. Until it is, install it as a
> [dev extension](https://zed.dev/docs/extensions/developing-extensions) from
> [Tusk-PHP/zed](https://github.com/Tusk-PHP/zed). It downloads the correct binary for your
> platform automatically.

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

The Zed extension doesn't add its own commands. Copy namespace, move to namespace and
copy reference are offered as code actions by the language server, in Zed's code-action
menu (`editor: toggle code actions`). `Restart Server` maps to Zed's built-in
`lsp: restart language servers` action.

## Neovim

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

## Other Editors

Any LSP client works. The server uses **stdio** with JSON-RPC 2.0:

```bash
tusk-php --transport stdio
```

## Binary Installation

**Download** from [GitHub Releases](https://github.com/Tusk-PHP/lsp/releases/latest) (Linux, macOS, Windows — amd64/arm64).

**Quick install** (Linux / macOS):

```bash
curl -fsSL https://raw.githubusercontent.com/Tusk-PHP/lsp/main/scripts/install.sh | bash
```

**From source** (requires Go 1.22+):

```bash
git clone https://github.com/Tusk-PHP/lsp.git tusk-php && cd tusk-php && make install
```
