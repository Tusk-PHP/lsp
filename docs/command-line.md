# Command line

Back to the [README](../README.md).

## `tusk-php check`

Reports the diagnostics your editor shows, for a whole project or the paths you
name, without an editor. It indexes the project the way the language server does:
the PHP version and extensions from `composer.json`, the framework, and
`.tusk-php.json`, including `diagnosticRules`.

```bash
tusk-php check                       # every project file, vendor/ left out
tusk-php check src/Http app/Models/User.php 'src/*/Console/*.php'
tusk-php check --root ../my-app --format json
tusk-php check --exclude Tests --exclude '*Test.php' src
```

Each finding is printed as `path:line:column: severity code: message`, with paths
relative to the root. A summary goes to standard error. Directories cover the
indexed PHP files under them, so paths in `excludePaths` stay out.

| Option | Default | Description |
|---|---|---|
| `--root` | current directory | The project to index. |
| `--format` | `text` | `text`, or `json` for an array of `{path, line, column, severity, code, message}`. |
| `--exclude` | | Leave out files matching a glob; repeat it for more. Without a `/` it matches any directory or file name in the path (`Tests`, `*Test.php`); with one, a path from the root (`src/Legacy`, `app/code/*/Test`). |
| `--write-baseline` | | Write every finding to this file. |
| `--baseline` | | Report only findings this file doesn't list. |

It exits with `1` when there's an error or a warning (hints and information don't
count), `0` when there's none, and `2` when it can't run.

### Baselines

To adopt it in CI on a project that already has findings, record them once:

```bash
tusk-php check --write-baseline .tusk-baseline.txt
```

Then check against the file. Only findings it doesn't list are printed, and they
fail the run whatever their severity:

```bash
tusk-php check --baseline .tusk-baseline.txt
```

A baseline lists each finding's path, code and message, without its position, so
it still matches after code moves. Write it again after fixing findings to drop
them from it; the summary says how many it lists that are no longer found.
