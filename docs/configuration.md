# Configuration

Back to the [README](../README.md).

## Editor Settings (VS Code)

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

## Project Configuration

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
  "coverageReportPath": "",
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

## Test coverage

Hover cards and the `php_coverage` MCP tool read the Clover report your tests last wrote. Tusk PHP never runs your tests, so create the report yourself:

```bash
vendor/bin/phpunit --coverage-clover clover.xml
```

Pest and `php artisan test` take the same flag. You need the PCOV or Xdebug extension for any of them.

The report is picked up from the `<clover outputFile="...">` in any `phpunit*.xml*` file in your project root, or from `clover.xml`, `build/logs/clover.xml`, `build/coverage/clover.xml`, `coverage/clover.xml` or `tests/coverage/clover.xml`. If several exist, the newest wins. To use another location, set `coverageReportPath` in `.tusk-php.json`, either absolute or relative to the project root; discovery is then skipped.

Reports written inside Docker or Sail work without any path mapping. A new report is noticed within a few seconds of your next hover.
