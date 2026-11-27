# Diagnostics

Back to the [README](../README.md).

Tusk PHP reports problems in your code from three sources:

| Source | What it reports | When it runs | Shown as |
|---|---|---|---|
| **Built-in checks** | Unknown classes and members, undefined variables, wrong argument counts and more ([full list](#built-in-rules)) | As you type, on the file you're editing | `tusk-php` |
| **PHPStan** | Whatever your PHPStan setup reports | On save, on the file you saved | `phpstan` |
| **Laravel Pint** | Code that Pint would reformat | On save, on the file you saved | `pint` |

All three are on by default. PHPStan and Pint run only when the server can find
them, so a project without them just gets the built-in checks.

## Built-in checks

The built-in checks run inside the server, with no extra process. After an edit
the server waits 200 ms of no typing, then checks the open file only, so their
cost grows with the size of that file, not with the project.

A few rules need types or Eloquent models, so they run when you save the file
instead (marked *on save* below).

While the project is still being indexed, the server holds back the rules that
need to know every class in the project (`unknown-class`, `unknown-function`,
`unknown-member`, `invalid-override`), so you don't see warnings that would
disappear a moment later.

### Built-in rules

| Code | Reports | Default severity |
|---|---|---|
| `syntax-error` | Code that doesn't parse | Error |
| `abstract-in-concrete` | An abstract method in a class that isn't declared abstract, which PHP refuses to compile | Error |
| `unknown-class` | A class, interface, trait or enum that isn't defined in the project, its Composer dependencies or PHP | Warning |
| `unknown-function` | A function that isn't defined anywhere | Warning |
| `unknown-member` | A method, property or constant that the receiver's class doesn't have | Warning |
| `builtin-unavailable` | A PHP function or class that exists, but not in your project's PHP version or installed extensions (from `composer.json`) | Warning |
| `undefined-variable` | A variable used before it's assigned | Warning |
| `unused-variable` | A variable assigned and never read | Hint |
| `argument-count-mismatch` | A call with too few or too many arguments | Warning |
| `unknown-named-argument` | A named argument the function, method or constructor has no parameter for, such as `new User(nmae: 'x')`, which PHP throws on | Error |
| `unused-import` | A `use` statement nothing in the file uses | Hint |
| `unused-private-method` | A private method nothing calls | Info |
| `unused-private-property` | A private property nothing reads | Hint |
| `unused-private-constant` | A private class constant nothing reads | Hint |
| `unreachable-code` | Code after a `return`, `throw`, `exit` and the like | Warning |
| `redundant-union-member` | A union type with a duplicate member, or a member already covered by another (`int\|mixed`) | Warning |
| `deprecated` | Use of a PHP function deprecated in your PHP version, or of anything marked `#[\Deprecated]` | Warning |
| `unused-result` | A call to something marked `#[\NoDiscard]` whose result is thrown away (cast to `(void)` to do that on purpose) | Warning |
| `invalid-override` | A method with `#[\Override]` that doesn't override anything, which PHP refuses to compile | Error |
| `redundant-nullsafe` | `?->` on a value that can't be null (*on save*) | Info |
| `unknown-column` | An Eloquent query naming a column the model doesn't have, such as `where('nmae', …)` (*on save*) | Warning |
| `unknown-relation` | An Eloquent query naming a relation the model doesn't have, such as `with('comments')` (*on save*) | Warning |
| `unknown-validation-rule` | A Laravel validation rule name that neither the framework nor your service providers define, such as `'requierd\|email'` in `$request->validate()`, `Validator::make()` or a form request's `rules()`. Rules built at run time are not checked, and nothing is reported while a provider registers a rule under a name the server can't read (*on save*) | Warning |
| `unclosed-directive` | In a Blade view, a block directive such as `@if` or `@foreach` with no matching `@end…` | Error |
| `unknown-view` | In a Blade view, an `@include`, `@extends` or other view name that has no view file | Warning |
| `unknown-component` | In a Blade view, an `<x-…>` component that the project and its packages don't define | Warning |

## PHPStan

When you save a PHP file, the server runs PHPStan on that file only:

```bash
phpstan analyse --error-format=json --no-progress <file>
```

It runs from the project root, so PHPStan uses its own result cache and your
configuration. It never runs while you type.

- **Which PHPStan:** `vendor/bin/phpstan` in the project, otherwise `phpstan`
  on your `PATH`. Set `phpstanPath` to use another one.
- **Which configuration:** the first of `phpstan.neon`, `phpstan.neon.dist` and
  `phpstan.dist.neon` in the project root. Set `phpstanConfig` to use another file.
- **Level:** your configuration decides. `phpstanLevel` overrides it.

PHPStan's findings are shown as errors and stay until the next save of that file.

## Laravel Pint

When you save a PHP file, the server runs `pint --test` on that file and marks
what Pint would change. It doesn't change your file: format with Pint as you
normally do. Blade and other templates are skipped.

It uses `vendor/bin/pint`, otherwise `pint` on your `PATH`, and Pint's own
`pint.json`. `pintPath` and `pintConfig` point it elsewhere.

## Turning diagnostics off

| To turn off | VS Code setting | `.tusk-php.json` or Zed key |
|---|---|---|
| Everything (built-in checks, PHPStan and Pint) | `tuskPhpLsp.diagnostics.enable` | `diagnosticsEnabled` |
| PHPStan | `tuskPhpLsp.diagnostics.phpstan.enable` | `phpstanEnabled` |
| Pint | `tuskPhpLsp.diagnostics.pint.enable` | `pintEnabled` |
| One built-in rule | none, use `.tusk-php.json` | `diagnosticRules` |

To keep PHPStan and drop the built-in checks, leave diagnostics on and turn off
the built-in rules one by one in `diagnosticRules`.

To silence a single finding, or every finding in some files, see
[Ignoring a finding](#ignoring-a-finding).

### Rule severity

`diagnosticRules` takes each rule's code with one of these values:

| Value | Effect |
|---|---|
| `true` | On, at its default severity |
| `false` or `"off"` | Off |
| `"hint"`, `"info"`, `"warning"`, `"error"` | On, at that severity |

```json
{
  "diagnosticRules": {
    "unused-variable": "off",
    "unknown-member": "error",
    "unused-import": "warning"
  }
}
```

Rules you don't list stay on at their default severity. PHPStan and Pint
findings can't be re-graded this way; use their own configuration.

## Ignoring a finding

To silence one finding, put a `@tusk-ignore` comment on the line or on its own
line above it, naming the codes to ignore. A comment on its own line covers the
next line of code and everything between, so markers can be stacked and can sit
above a method's docblock and attributes. It reaches across at most one blank
line; a marker followed by two blank lines covers nothing. The marker starts
the comment (or, in a docblock, one of its lines), so a note such as
`// TODO: drop @tusk-ignore` silences nothing:

```php
// @tusk-ignore unknown-member
$legacy->magicCall();

$value = $row->total; // @tusk-ignore unknown-member, undefined-variable (set by an include)
```

Codes are separated by commas, and each may be followed by a reason in
parentheses; anything after the list is ignored, as is punctuation right after a
code (`unused-variable: read by an include`). A bare `@tusk-ignore` ignores every
finding on that line, and so does `@tusk-ignore: reason`. In a Blade view, use
a Blade comment: `{{-- @tusk-ignore unknown-view --}}`; inside
`@php … @endphp`, use a PHP comment. A directive or `{{ … }}` echo that spans
several lines counts as one line: a marker above it, or after its end, covers
all of it, and the quick fix puts the marker above the line it starts on. This
holds for the directives and conditionals your service providers register
(`Blade::directive`, `Blade::if`) as well as Blade's own.

Findings from external tools are named with the tool in front of their code:
`@tusk-ignore phpstan:argument.type` ignores one PHPStan identifier, and
`@tusk-ignore phpstan` ignores all of PHPStan's findings on the line. Pint
reports a line once for all the fixers that would change it; `pint:single_quote`
ignores that line when `single_quote` is one of them. `tusk-php` names every
Tusk PHP finding. The "Ignore … on this line" quick fix writes the comment for
you, above any docblock and attributes, offering one action per Pint fixer. For
PHPStan it writes PHPStan's own `@phpstan-ignore` at the end of the line, the
one place PHPStan reads it however the declaration is laid out, so PHPStan in
CI agrees.

Comments already written for PHPStan work as they are, with PHPStan's rules for
which line they cover: a `@phpstan-ignore` on its own line and
`@phpstan-ignore-next-line` cover the very next line only, not code below a
docblock, another comment or a blank line. `@phpstan-ignore-line` and
`@phpstan-ignore-next-line` silence PHPStan's and Tusk PHP's findings on their
line, but not Pint's.
`@phpstan-ignore <identifier>, …` silences those PHPStan identifiers, and the
Tusk PHP finding that matches each identifier too, for example
`method.notFound` → `unknown-member`, `variable.undefined` →
`undefined-variable`, `arguments.count` → `argument-count-mismatch` and
`deadCode.unreachable` → `unreachable-code`. A bare `@phpstan-ignore`, which
PHPStan rejects, silences nothing.

To ignore findings across files, list rules under `diagnosticIgnore` in
`.tusk-php.json`. Each rule has any of `code`, `path` and `message` (a regular
expression); a finding is ignored when it matches everything its rule gives.
`path` is a glob relative to the project root, read like a `.gitignore` entry:
`**` matches any number of directories, a directory covers everything below it,
a pattern without a `/` (`*.blade.php`) matches at any depth, and a leading `/`
anchors it to the root. `./` in front and Windows `\` separators are accepted:

```json
{
  "diagnosticIgnore": [
    { "path": "app/Legacy/**" },
    { "code": "unknown-member", "message": "on 'LegacyModel'$" },
    { "code": "phpstan:missingType.iterableValue", "path": "tests/**" }
  ]
}
```

A rule with an unknown `code` (a rule that doesn't exist, or a tool other than
`phpstan` and `pint`) or an invalid `path` or `message` is skipped, and the
editor shows which rule and why; the rest of the configuration still applies. Both kinds of suppression apply in the editor and to
`tusk-php check`.

## Where settings come from

The server reads its settings once, when it starts, in this order. Each
step overrides the one before it:

1. The server's defaults.
2. `.tusk-php.json` in the project root: settings for everyone who opens the project.
3. Your editor's settings: VS Code's `tuskPhpLsp.*` settings, or Zed's
   `lsp.tusk-php.initialization_options`.

So a setting in your editor wins over the same setting in `.tusk-php.json`.
Use `.tusk-php.json` for what the whole team should share, and your editor for
your own preferences.

> **VS Code:** the extension currently sends `diagnostics.enable`,
> `diagnostics.phpstan.enable` and `diagnostics.pint.enable` even when you
> haven't changed them. Until that's fixed, `diagnosticsEnabled`,
> `phpstanEnabled` and `pintEnabled` in `.tusk-php.json` have no effect in VS
> Code: turn these off in VS Code's settings instead. `diagnosticRules` works
> from `.tusk-php.json` as described.

### Zed

Zed reads the same keys as `.tusk-php.json`, under `initialization_options`
in its `settings.json`:

```json
{
  "lsp": {
    "tusk-php": {
      "initialization_options": {
        "phpstanEnabled": false,
        "diagnosticRules": { "unused-variable": "off" }
      }
    }
  }
}
```

### Applying a change

Settings are read when the server starts, so restart it after changing one:
**Tusk PHP: Restart Server** in VS Code, or `lsp: restart language servers`
in Zed.

## Outside the editor

`tusk-php check` reports the built-in checks from the command line, with the
same `diagnosticRules` from `.tusk-php.json`; see
[Command line](command-line.md). The MCP server's `php_diagnostics` tool also
reports the built-in checks; neither runs PHPStan or Pint.
