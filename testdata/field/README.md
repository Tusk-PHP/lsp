# Field baselines

What `tusk-php check` reports on real codebases at pinned versions: the
source of four frameworks (Laravel's framework, Symfony, WordPress and
Magento 2) and a reference app per framework, the kind of project a user
opens (the Filament demo, Sylius, WooCommerce on WordPress, Mage-OS). CI fails when a change
adds a finding to one of them or stops reporting one, so false warnings can't
come back unnoticed and fixes show up in the pull request as removed lines.

- `manifest.json`: each codebase's repository, commit, Composer lock, the paths
  checked and the paths left out (`--exclude`). A reference app has `fetch`
  (its script in `scripts/`) and `root` (where that script puts it) instead.
- `locks/`: lock files for the codebases that don't commit one, so every run
  installs the same dependencies. Magento 2 uses its own. `locks/mageos/` holds
  the Mage-OS project's `composer.json` and lock.
- `<name>.baseline`: the findings, one per line as path, code and message.
- `_repos/`: the checkouts, fetched by the script (ignored by git).

```bash
make field-baseline                 # check them all (NAME=symfony for one)
make field-baseline-update          # rewrite the baselines after an intended change
```

Look at every added or removed line before committing an updated baseline: an
added one is a new warning users will see.

To move a codebase to a newer commit, change its `sha`, replace its lock in
`locks/` with one made by `composer update` at that commit, then run
`make field-baseline-update NAME=<name>` and review the diff. Symfony's
monorepo needs its branch version in `rootVersion` to install.

## Reference apps

| name | what | pinned at | paths checked |
|---|---|---|---|
| `filament` | Filament demo (Laravel, Livewire, Blade) | commit in `scripts/fetch-filament.sh` | `app` |
| `sylius` | Sylius-Standard (Symfony, Doctrine, API Platform) | v2.3.1, `locks/sylius.lock` | `src`, the Sylius bundles and components in `vendor/sylius/sylius` |
| `woocommerce` | WooCommerce 11.3.1 on WordPress 7.1.5, laid out as a site | release zips, by SHA-256 | the plugin, without its `vendor`, `packages` and tests |
| `mageos` | Mage-OS 4.0.0 with the Luma sample data | `locks/mageos/` | `vendor/mage-os`, without tests |

`make reference-apps` fetches all four into `testdata/<name>` (gitignored).
To move one to a newer release, change the pin in its fetch script (Sylius and
Mage-OS: run the script with `--lock` to re-resolve the lock), then update the
baseline and look at the diff. The same apps are benchmark workspaces
(`bench/suite/<name>.json`), held to `bench/passmarks.json` by
`make bench-reference`.
