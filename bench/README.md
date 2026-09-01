# Benchmarking tusk-php against other PHP language servers

`lspbench` asks several PHP language servers the same questions an editor would
ask, at the same caret positions, in the same workspaces — then reports how fast
each one starts, how much memory it holds, how quickly it answers, and how many
answers are right.

It talks nothing but plain LSP over stdio, so adding a server is a JSON entry,
not code.

## Quick start

```bash
bash bench/tools/install.sh      # fetch the other servers into bench/.tools/
go run ./bench/lspbench -list    # what is installed, and what will be asked
go run ./bench/lspbench          # run everything, write a report
```

Results land in `bench/results/<timestamp>/`: `results.json` (every case, every
timing) and `report.md` (tables). Both are gitignored.

Useful flags:

```bash
go run ./bench/lspbench -servers tusk-php            # one server
go run ./bench/lspbench -workspaces symfony          # one workspace
go run ./bench/lspbench -features hover,completion   # one kind of question
go run ./bench/lspbench -cases eloquent -v           # cases whose id matches, verbosely
go run ./bench/lspbench -repeat 20                   # more samples per case
```

## Who is being compared

| server | what it is | how it is run |
|---|---|---|
| **tusk-php** | this repository | `build/tusk-php --stdio` |
| **Intelephense** | the de-facto default for VS Code; closed source, free tier | npm package, run with no licence key |
| **Phpactor** | open-source PHP server, also the engine behind several Vim/Neovim setups | official phar |
| **php-lsp** | Rust server by jorgsowa, MIT | prebuilt release binary, stdio with no arguments |
| **PHPantom** | Rust server, MIT | prebuilt release binary, `--stdio` |
| **Psalm** | static analyser with a language-server front end | off by default: it will not start without a `psalm.xml` in the workspace |

`install.sh` verifies a downloaded release against its published `.sha256`
sidecar when the project ships one (php-lsp does; PHPantom does not yet), and
refuses to install on a mismatch.

Deliberately absent: **php-language-server** (felixfbecker) is unmaintained and
does not run on modern PHP; **Serenata** is archived; **DEVSENSE PHP Tools** is
proprietary and not distributed as a standalone stdio server. If any of that
changes, they are three lines of `servers.json` each.

## What is measured

**Start-up.** Two numbers, both from process spawn:

- *initialize* — the response to the `initialize` request. This is only the
  handshake; every server answers it long before it can answer a question.
- *ready* — the first moment the server correctly answers a cross-file question
  (the `readyProbe` case of each workspace, polled every 250 ms). This is the
  only start-up figure that means the same thing everywhere, because servers
  signal "finished indexing" differently, or not at all. Where a server does
  emit its own signal, it is recorded alongside for reference.

**Footprint.** Peak RSS of the server *and its children*, sampled four times a
second, reported both at the moment of readiness and across the whole run, plus
total CPU seconds. A Node or PHP server carries its runtime in these numbers.
That is a real cost to the user, but it is not a like-for-like comparison of
indexing strategy — read it as "what the machine pays", not "who wrote better
code".

**Latency.** Per case: one warm-up call, then five timed calls, reported as
p50/p95 per feature. Measured after readiness, so this is steady-state editing,
not cold start.

**Correctness.** Each case asserts what the answer must contain, loosely enough
that formatting differences do not matter:

- *hover* / *signatureHelp* — the flattened text must contain given substrings
- *definition* / *typeDefinition* / *implementation* — some returned target must
  be in the expected file, on a line containing the expected declaration
  (±2 lines, because servers disagree about whether a definition points at the
  docblock, the attribute, or the declaration)
- *completion* — the expected labels must be offered; the report records their
  rank and the total item count
- *references* — a minimum count plus the files that must appear
- *documentSymbol* / *workspaceSymbol* — the expected names must appear
- *diagnostics* — a finding on the expected line whose message uses any of
  several accepted wordings, or, for the clean file, no error or warning at all

A case the server does not advertise support for is scored `n/a` and left out of
the denominator, never counted as a failure. A case where the server *did*
advertise the capability and then failed to answer — typically no reply at all
before the request timeout — is scored `err`, and the run records both the
server's advertised capability list and the tail of its stderr so the two can be
told apart afterwards.

## Fairness rules

These are the things that make the numbers mean anything. They are enforced by
the harness, not by convention:

1. **Same questions, same positions.** Every server gets the identical caret,
   computed from an anchor string rather than a line number, so the suite does
   not silently drift when a fixture is edited.
2. **Cold cache, every run.** Each server's cache directory lives under
   `bench/.work/<server>/<workspace>` and is deleted before the run. The harness
   refuses to delete anything outside `bench/.work`.
3. **Warm page cache.** Every PHP file in the workspace is read once before the
   first server starts, so whoever runs first does not pay for cold I/O.
4. **Sequential.** One server at a time; no CPU contention.
5. **Editor-shaped client.** The harness advertises the capabilities VS Code
   advertises and answers the server→client requests an editor answers
   (`workspace/configuration`, `window/workDoneProgress/create`,
   `client/registerCapability`). A server that stalls waiting for a reply is a
   harness bug, not a server result.
6. **No paid tiers.** Intelephense runs without a licence key. Its premium
   features (rename, code actions, find implementations) are therefore scored
   `n/a`, not failed.
7. **Diagnostics get `didSave` as well as `didOpen`,** because some servers only
   analyse on save, and the question is what the analysis finds — not which
   notification a server listens for.

## Workspaces

| name | source | size | what it exercises |
|---|---|---|---|
| `laravel` | `testdata/laravel` | ~9.7k PHP files with full `vendor/` | large-workspace indexing, Eloquent, framework depth |
| `symfony` | `testdata/symfony` | ~1.1k PHP files with `vendor/` | DI, autowired services, entities, PHP builtins |
| `diagnostics` | `bench/workspaces/diagnostics` | 6 files, no `vendor/` | one deliberate defect per file, plus a clean file to catch false positives |

The first two are the repository's existing test fixtures, so the suite tracks
the same code the unit tests do. The third is benchmark-only and lives here
rather than in `testdata/` precisely because it contains code that is *meant* to
be wrong — it must never reach the conformance corpus.

## Adding a case

Cases live in `bench/suite/*.json`. A caret is an anchor string plus a rune
offset from its start, so it survives edits above it:

```json
{
  "id": "def.eloquent-relation",
  "feature": "definition",
  "desc": "property access that resolves to an Eloquent relation method",
  "file": "app/Http/Controllers/CategoryController.php",
  "anchor": "$category->products;",
  "offset": 13,
  "expect": { "targetFile": "app/Models/Category.php", "targetSymbol": "function products" },
  "tags": ["laravel", "eloquent"]
}
```

Two rules of thumb, learned from writing the first suite:

- **When every server fails a case, suspect the case.** Sometimes it is a real
  shared gap (nobody resolves Eloquent's magic statics without a helper file),
  and sometimes the caret is in the wrong place. The failure detail includes a
  sample of what was actually returned so the two are easy to tell apart.
- **Keep framework depth in its own cases.** A case whose receiver is typed
  through `Model::first()` is testing Laravel knowledge, not member completion.
  Mixing them turns one score into two questions.

## Adding a server

```json
{
  "id": "my-server",
  "name": "My Server",
  "license": "MIT",
  "cmd": ["php", "{{tools}}/my-server.phar", "language-server"],
  "versionCmd": ["php", "{{tools}}/my-server.phar", "--version"],
  "env": { "XDG_CACHE_HOME": "{{storage}}" },
  "initializationOptions": { "storagePath": "{{storage}}" },
  "resetPaths": ["{{storage}}"],
  "readyTimeoutSec": 300
}
```

`{{root}}`, `{{bench}}`, `{{tools}}`, `{{workspace}}` and `{{storage}}` are
expanded in `cmd`, `env`, `initializationOptions` and `resetPaths`.

## Reporting numbers publicly

Publish `report.md` as-is: it carries the machine, the OS, the PHP and Node
versions, and the resolved version of every server, which is most of what makes
a benchmark reproducible. Two things to state plainly alongside it:

- the case suite was written in this repository, and a suite written by someone
  else would emphasise different things;
- memory figures include each server's language runtime.

Both are honest to say, and saying them is cheaper than being corrected.
