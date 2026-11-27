#!/usr/bin/env bash
# fetch-woocommerce.sh: put WooCommerce on a pinned WordPress into
# testdata/woocommerce, the WordPress reference app.
#
# Usage:
#   bash scripts/fetch-woocommerce.sh
#
# The workspace is laid out like a real site: WordPress at the root and
# WooCommerce (GPL-3.0) in wp-content/plugins/woocommerce, with the vendor/
# its release ships. Both come from the official release zips, checked against
# the SHA-256 sums below; nothing is installed with Composer.
#
# To move to newer releases, change the versions and sums (shasum -a 256 of
# the zips), run this script, then look at what changed in
# testdata/field/woocommerce.baseline and bench/suite/woocommerce.json.
#
# Needs curl, shasum and unzip.

set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib/reference-app.sh"
log() { echo "[fetch-woocommerce] $*"; }

WP_VERSION="7.1.5"
WP_SHA256="cd44160a3fcce179fe170331132fcfc05e84e2f9284e2aad38df2aea6edaeab1"
WC_VERSION="11.3.1"
WC_SHA256="1502600c738840f7f9ebfa2f11e1aaff087221a5fc5a2006c51be2d874ae2a09"

DEST="${REPO_ROOT}/testdata/woocommerce"
CACHE="${REPO_ROOT}/testdata/.downloads"
STAMP="wordpress-${WP_VERSION} woocommerce-${WC_VERSION}"

need curl shasum unzip
# WordPress states no PHP version; WooCommerce supports 7.4 and up.
PHP_VERSION="8.4"

if [[ "$(cat "${DEST}/.fetch-stamp" 2>/dev/null || true)" == "${STAMP}" ]]; then
  pin_php "${DEST}" "${PHP_VERSION}"
  log "up to date"
  exit 0
fi

mkdir -p "${CACHE}"
download "https://wordpress.org/wordpress-${WP_VERSION}.zip" "${WP_SHA256}" "${CACHE}/wordpress-${WP_VERSION}.zip"
download "https://downloads.wordpress.org/plugin/woocommerce.${WC_VERSION}.zip" "${WC_SHA256}" "${CACHE}/woocommerce-${WC_VERSION}.zip"

log "unpacking"
rm -rf "${DEST}" "${DEST}.part"
mkdir -p "${DEST}.part"
unzip -q "${CACHE}/wordpress-${WP_VERSION}.zip" -d "${DEST}.part"
mv "${DEST}.part/wordpress" "${DEST}"
rmdir "${DEST}.part"
unzip -q "${CACHE}/woocommerce-${WC_VERSION}.zip" -d "${DEST}/wp-content/plugins"
pin_php "${DEST}" "${PHP_VERSION}"
echo "${STAMP}" > "${DEST}/.fetch-stamp"
log "done"
