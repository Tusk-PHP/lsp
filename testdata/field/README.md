# Field baselines

What `tusk-php check` reports on four real codebases, at pinned commits:
Laravel's framework, Symfony, WordPress and Magento 2. CI fails when a change
adds a finding to one of them or stops reporting one, so false warnings can't
come back unnoticed and fixes show up in the pull request as removed lines.

- `manifest.json`: each codebase's repository, commit, Composer lock, the paths
  checked and the paths left out (`--exclude`).
- `locks/`: lock files for the codebases that don't commit one, so every run
  installs the same dependencies. Magento 2 uses its own.
- `<name>.baseline`: the findings, one per line as path, code and message.
- `_repos/`: the checkouts, fetched by the script (ignored by git).

```bash
make field-baseline                 # check all four (NAME=symfony for one)
make field-baseline-update          # rewrite the baselines after an intended change
```

Look at every added or removed line before committing an updated baseline: an
added one is a new warning users will see.

To move a codebase to a newer commit, change its `sha`, replace its lock in
`locks/` with one made by `composer update` at that commit, then run
`make field-baseline-update NAME=<name>` and review the diff. Symfony's
monorepo needs its branch version in `rootVersion` to install.
