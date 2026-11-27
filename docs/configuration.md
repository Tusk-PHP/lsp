# Configuration

Back to the [README](../README.md).

## Editor Settings (VS Code)

| Setting | Default | Description |
|---------|---------|-------------|
| `tuskPhpLsp.enable` | `true` | Enable/disable the extension |
| `tuskPhpLsp.executablePath` | `""` | Custom path to the binary |
| `tuskPhpLsp.phpVersion` | `"8.6"` | Target PHP version (`7.4`, `8.0`-`8.6`), used when `composer.json` requires none. It decides which builtins exist and how files are parsed; without it the `php` on your PATH decides; with neither, builtins follow `8.6` and any version from `7.4` to `8.6` may be parsed. With a `composer.json` PHP constraint, builtins follow the oldest version it admits, and files are parsed with the versions it admits. Where several versions are possible, each file is first parsed as the newest one it has no syntax error on, or as the newest when it has errors on all of them; after an edit it keeps that version, and moves to the newest once that parses it without errors. |
| `tuskPhpLsp.framework` | `"auto"` | Framework: `auto`, `laravel`, `symfony`, `magento`, `none` |
| `tuskPhpLsp.containerAware` | `true` | Enable DI container analysis |
| `tuskPhpLsp.diagnostics.enable` | `true` | Enable diagnostics |
| `tuskPhpLsp.diagnostics.phpstan.enable` | `true` | Run PHPStan on save |
| `tuskPhpLsp.diagnostics.pint.enable` | `true` | Run Laravel Pint on save |
| `tuskPhpLsp.coverage.reportPath` | `""` | Clover report to read, absolute or relative to the workspace. Empty finds it automatically. |
| `tuskPhpLsp.maxIndexFiles` | `0` | Maximum workspace PHP files to index; `0` indexes all of them. Set a limit only if the server uses too much memory: indexing all of Magento 2 (about 25,000 files) takes about 600 MB. Composer dependencies aren't counted. The WebAssembly build (vscode.dev) defaults to `10000` to stay within browser memory. |
| `tuskPhpLsp.excludePaths` | `["vendor", "node_modules", ".git", "storage", "var/cache"]` | Directories to skip when indexing. An entry without a `/` matches a directory of that name anywhere in the workspace (`vendor`); an entry with one matches the directory at that path from the workspace root (`var/cache`, not `src/var/cache`). A leading `/` or `./` anchors an entry to the workspace root (`/build` skips the `build` directory at the root, not `src/build`); a trailing `/` is ignored. Composer dependencies are read from Composer's autoloader either way. |
| `tuskPhpLsp.phpManual.locale` | `""` | Locale for php.net manual links (`en`, `de`, `es`, `fr`, `it`, `ja`, `pt_BR`, `ru`, `tr`, `zh`). Empty = English. |
| `tuskPhpLsp.phpManual.openOnDefinition` | `false` | Cmd+Click on a built-in opens its php.net page in your browser. |

## Project Configuration

Create `.tusk-php.json` in your project root for settings everyone who opens the project shares. Your editor's settings override the same keys in this file; see [Where settings come from](diagnostics.md#where-settings-come-from).

```json
{
  "phpVersion": "8.6",
  "framework": "auto",
  "containerAware": true,
  "diagnosticsEnabled": true,
  "excludePaths": ["vendor", "node_modules", ".git"],
  "maxIndexFiles": 0,
  "php_manual_locale": "",
  "php_manual_open_on_definition": false,
  "coverageReportPath": "",
  "diagnosticRules": {
    "unused-import": true,
    "unused-private-method": true,
    "unused-private-property": true,
    "unreachable-code": true,
    "invalid-override": true,
    "deprecated": true,
    "unused-result": true,
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
  },
  "autoImport": {
    "aliasStyle": "suffix"
  }
}
```

All diagnostic rules are enabled by default. Set a rule to `false` (or `"off"`) to disable it, or to `"error"`, `"warning"`, `"info"` or `"hint"` to change its severity. A rule name or severity that doesn't exist is an error: the editor shows it, and `tusk-php check` stops with it. Every rule, and how to turn off PHPStan and Pint, is in [Diagnostics](diagnostics.md). Framework is auto-detected from `artisan` (Laravel), `bin/console` (Symfony), `bin/magento` (Magento 2), or `composer.json`.

### What applies without a restart

The PHP version and extensions follow `composer.json` and `.tusk-php.json` while the server runs: save either file and builtins, completion, hover and `builtin-unavailable` use the new profile, and open files are checked again. The same happens when the client sends `workspace/didChangeConfiguration` with settings shaped like `initializationOptions` (Zed and Neovim send their `settings` this way). Those settings apply over `initializationOptions` instead of replacing them, and settings with no Tusk PHP option in them, such as the empty object Zed sends when its `settings` are unset, change nothing; a client that sends no settings still makes the server read both files again. A `.tusk-php.json` that can't be read, such as a half-edited save with a syntax error, is reported and the last version that could be read stays in use. Every other option is read once, at startup, and needs a restart.

PHP files created, changed or deleted outside the editor (a branch switch, a code generator, `rm`) reach the index too, except in directories under `excludePaths`. The server asks the client to watch them when the client lets servers register file watchers, as VS Code, Zed and Neovim do. Deleting or renaming a directory counts too: its files leave the index, or are indexed at the new place. A file open in the editor keeps the editor's text until it is closed; closing it without saving brings back the text on disk.

## Auto-import

Completing a class from another namespace adds its `use` statement. When the class's short name is already in use in the file (by another import, a class declared in the file, or a class in the file's own namespace, ignoring case), the new import gets an alias instead, so the file never ends up with two `use … User;` lines. Imports already in the file are never renamed.

`autoImport.aliasStyle` sets how that alias is made. It is read from `.tusk-php.json` and from the client's `initializationOptions`:

| Value | Adds | Inserts, for `App\Models\User` |
|---|---|---|
| `suffix` (default) | `use App\Models\User as UserModel;` | `UserModel` |
| `prefix` | `use App\Models\User as ModelUser;` | `ModelUser` |
| `fullyQualified` | nothing | `\App\Models\User` |

The alias is the class name plus the namespace segment above it in the singular (`Models` → `Model`, `Categories` → `Category`, `Statuses` → `Status`; `Auth` stays `Auth`). If that alias is taken too, the next segment up is added (`Illuminate\Foundation\Auth\User` becomes `UserAuth`, then `UserFoundationAuth`). When every segment is used up, the class is written fully qualified. A class that is already imported, under an alias or not, is inserted by the name it has in the file.

## Test coverage

Hover cards and the `php_coverage` MCP tool read the Clover report your tests last wrote. Tusk PHP never runs your tests, so create the report yourself:

```bash
vendor/bin/phpunit --coverage-clover clover.xml
```

Pest and `php artisan test` take the same flag. You need the PCOV or Xdebug extension for any of them.

The report is picked up from the `<clover outputFile="...">` in any `phpunit*.xml*` file in your project root, or from `clover.xml`, `build/logs/clover.xml`, `build/coverage/clover.xml`, `coverage/clover.xml` or `tests/coverage/clover.xml`. If several exist, the newest wins. To use another location, set `coverageReportPath` in `.tusk-php.json`, either absolute or relative to the project root; discovery is then skipped.

Reports written inside Docker or Sail work without any path mapping. A new report is noticed within a few seconds of your next hover.

In VS Code, the same report also appears in the built-in Test Coverage view and editor gutter, and new reports load on their own. Files that changed since the report are hidden until you re-run your tests. If the [PHPUnit extension](https://marketplace.visualstudio.com/items?itemName=recca0120.vscode-phpunit) is installed, its coverage is left alone until you run **Tusk PHP: Show Test Coverage**. Set `tuskPhpLsp.coverage.reportPath` to point at a report the server doesn't find.

Other clients can use the same data. When the server advertises `experimental.tuskCoverage`:

- `tusk/coverageReport` takes no params and returns `{ path, generated, files: [{ uri, stale, coveredLines, executableLines, coveredMethods, methods }] }`, or `null` with no report. Only files on this machine are listed.
- `tusk/fileCoverage` takes `{ textDocument: { uri } }` and returns `{ uri, stale, lines: [{ line, hits }], methods: [{ name, line, endLine, hits, covered, executable }] }`, or `null` when the file isn't in the report. Lines are 0-based, `endLine` is the method's last executable line, and anything not listed is unknown rather than untested.
- `tusk/coverageChanged` is a notification with no params, sent when a report appears, changes or goes away. Set the `coverageNotifications` initialization option to `true` to get it.
