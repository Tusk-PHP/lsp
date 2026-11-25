<!-- See CONTRIBUTING.md for the pull request checklist. Mark the PR as a draft while it's a work in progress. -->

## What and why

<!-- What this changes and why. Link the issue it closes: "Closes #123". If it's stacked on another PR, say "Stacked on #123" here. -->

## Release note

<!-- One or two sentences for users of the release: what changed for them. Write "None" for tests, refactors and contributor docs. Don't edit CHANGELOG.md: the maintainers write it from this note when they release. -->

## Checks

- [ ] `make test` passes
- [ ] `make conformance-pr` / `make field-baseline` pass, if the change touches what they cover
- [ ] `make wasm` builds
- [ ] New behaviour has a test that fails without it
- [ ] Performance numbers are included, if AGENTS.md's performance rules ask for them
