#!/usr/bin/env bash
# fetch-mageos.sh: install Mage-OS with the Luma sample data into
# testdata/mageos, the Magento 2 reference app.
#
# Usage:
#   bash scripts/fetch-mageos.sh          # install from the pinned lock
#   bash scripts/fetch-mageos.sh --lock   # re-resolve the lock after a version change
#
# Mage-OS (https://mage-os.org, OSL-3.0/AFL-3.0) is the community distribution
# of Magento Open Source. It installs from its public Composer repository, so
# no repo.magento.com keys are needed. A real project has Magento under
# vendor/, with di.xml preferences and plugins, layout XML and the base files
# (app/etc/di.xml, bin/magento) that the Magento installer plugin puts in place.
#
# testdata/field/locks/mageos/ holds the project's composer.json (the
# mage-os/project-community-edition template at VERSION, without its dev
# tools, plus the sample data modules) and the lock made from it.
#
# Needs composer (and curl/unzip for --lock).

set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib/reference-app.sh"
log() { echo "[fetch-mageos] $*"; }

VERSION="4.0.0"
DEST="${REPO_ROOT}/testdata/mageos"
PINNED="${LOCKS_DIR}/mageos"

need composer git
mkdir -p "${DEST}"

if [[ "${1:-}" == "--lock" ]]; then
  need curl jq
  log "resolving mage-os/project-community-edition ${VERSION}"
  tmp="$(mktemp -d)"
  curl -fsSL -o "${tmp}/project.zip" \
    "https://repo.mage-os.org/packages/mage-os/project-community-edition-${VERSION}.zip"
  unzip -q "${tmp}/project.zip" -d "${tmp}/project"
  # The dev tools (PHPUnit, MFTF, static analysers) aren't what a project's
  # own code is read against, and they double the install.
  jq --arg v "${VERSION}" '
    del(.["require-dev"], .["autoload-dev"])
    | .require += ([
        "bundle", "catalog-rule", "catalog", "cms", "configurable", "customer",
        "downloadable", "grouped-product", "msrp", "offline-shipping",
        "product-links", "review", "sales-rule", "sales", "swatches", "tax",
        "theme", "widget", "wishlist"
      ] | map({key: "mage-os/module-\(.)-sample-data", value: $v}) | from_entries)
    | .require["mage-os/module-sample-data"] = $v
  ' "${tmp}/project/composer.json" > "${DEST}/composer.json"
  rm -rf "${tmp}"
  rm -f "${DEST}/composer.lock"
  composer update --working-dir "${DEST}" --no-interaction --no-progress --no-install \
    --no-scripts --no-plugins --ignore-platform-reqs --quiet
  mkdir -p "${PINNED}"
  cp "${DEST}/composer.json" "${DEST}/composer.lock" "${PINNED}/"
fi

cp "${PINNED}/composer.json" "${PINNED}/composer.lock" "${DEST}/"
# Plugins stay on: magento/magento-composer-installer deploys the base files
# (app/, bin/, pub/, setup/) every Magento project has. Scripts stay off.
composer_install "${DEST}" --no-scripts
# The project's composer.json states no PHP version (the framework packages
# do, with ~8.3, ~8.4 or ~8.5); 8.4 matches the PHP the CI jobs run.
pin_php "${DEST}" "8.4"
log "done"
