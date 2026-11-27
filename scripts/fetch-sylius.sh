#!/usr/bin/env bash
# fetch-sylius.sh: check out Sylius into testdata/sylius, the Symfony and
# Doctrine reference app.
#
# Usage:
#   bash scripts/fetch-sylius.sh          # fetch and install from the pinned lock
#   bash scripts/fetch-sylius.sh --lock   # re-resolve the lock after moving SHA
#
# Sylius-Standard (https://github.com/Sylius/Sylius-Standard, MIT) is a real
# Symfony application: a large autowired container, Doctrine entities and
# repositories, routes from attributes and config, Twig, translations, API
# Platform resources. It doesn't commit a composer.lock, so the one in
# testdata/field/locks/sylius.lock pins every dependency.
#
# To move to a newer release, change SHA, run with --lock, then look at what
# changed in testdata/field/sylius.baseline and bench/suite/sylius.json.
#
# Needs git and composer.

set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib/reference-app.sh"
log() { echo "[fetch-sylius] $*"; }

REPO="https://github.com/Sylius/Sylius-Standard.git"
SHA="62d4258e7670a25afb93da4c9bd6ddb7518d7a60" # v2.3.1
DEST="${REPO_ROOT}/testdata/sylius"
LOCK="${LOCKS_DIR}/sylius.lock"

need git composer
git_checkout "${REPO}" "${SHA}" "${DEST}"

if [[ "${1:-}" == "--lock" ]]; then
  log "resolving dependencies"
  rm -f "${DEST}/composer.lock"
  composer update --working-dir "${DEST}" --no-interaction --no-progress --no-install \
    --no-scripts --no-plugins --ignore-platform-reqs --quiet
  cp "${DEST}/composer.lock" "${LOCK}"
fi

cp "${LOCK}" "${DEST}/composer.lock"
composer_install "${DEST}" --no-scripts --no-plugins
log "done"
