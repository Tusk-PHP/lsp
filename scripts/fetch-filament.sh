#!/usr/bin/env bash
# fetch-filament.sh: check out the Filament demo app into testdata/filament.
#
# Usage:
#   bash scripts/fetch-filament.sh
#
# The demo (https://github.com/filamentphp/demo, MIT) is the large Laravel
# workspace for the benchmark and for the framework tests: Filament panels,
# Livewire, Blade components and Eloquent models on a full vendor/. It is
# fetched rather than committed. The commit below pins it, and the demo's own
# composer.lock pins every dependency.
#
# A checkout and an install that already match the pin are reused, so running
# this again is cheap.
#
# To move to a newer commit, change SHA, run this script, then look at what
# changed in the tests and the benchmark suite.
#
# Needs git and composer.

set -euo pipefail

REPO="https://github.com/filamentphp/demo.git"
SHA="b2bd9ab3b0def3313799398f7fe7f94b9f210885" # branch 5.x, 2026-11-21

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEST="${REPO_ROOT}/testdata/filament"

for tool in git composer; do
  command -v "${tool}" >/dev/null || { echo "ERROR: ${tool} is required" >&2; exit 2; }
done

log() { echo "[fetch-filament] $*"; }

if [[ "$(git -C "${DEST}" rev-parse HEAD 2>/dev/null || true)" != "${SHA}" ]]; then
  log "fetching ${SHA:0:12}"
  rm -rf "${DEST}"
  mkdir -p "${DEST}"
  git -C "${DEST}" init --quiet
  git -C "${DEST}" remote add origin "${REPO}"
  git -C "${DEST}" fetch --quiet --depth 1 origin "${SHA}"
  git -C "${DEST}" -c advice.detachedHead=false checkout --quiet FETCH_HEAD
fi

# Install once per lock. Platform requirements are ignored because only the
# source is needed: nothing in the demo is run, so missing PHP extensions
# (Octane's, for example) don't matter.
stamp="$(git -C "${DEST}" hash-object composer.lock)"
if [[ -d "${DEST}/vendor" && "$(cat "${DEST}/vendor/.fetch-lock" 2>/dev/null || true)" == "${stamp}" ]]; then
  log "up to date"
  exit 0
fi
log "installing Composer dependencies"
composer install --working-dir "${DEST}" --no-interaction --no-progress --prefer-dist \
  --no-scripts --no-plugins --ignore-platform-reqs --quiet
echo "${stamp}" > "${DEST}/vendor/.fetch-lock"
log "done"
