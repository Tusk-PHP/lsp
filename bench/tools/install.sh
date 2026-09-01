#!/usr/bin/env bash
# install.sh — fetch the language servers lspbench compares against.
#
# Everything lands in bench/.tools/ (gitignored). Nothing is installed globally
# and nothing outside bench/ is touched, so removing bench/.tools undoes it all.
#
# Usage:
#   bash bench/tools/install.sh              # intelephense + phpactor
#   bash bench/tools/install.sh --with-psalm # also fetch Psalm (disabled by default)
#   bash bench/tools/install.sh --check      # report what is installed

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCH_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${BENCH_DIR}/.." && pwd)"
TOOLS_DIR="${BENCH_DIR}/.tools"

INTELEPHENSE_VERSION="${INTELEPHENSE_VERSION:-latest}"
PHPACTOR_VERSION="${PHPACTOR_VERSION:-latest}"
PSALM_CONSTRAINT="${PSALM_CONSTRAINT:-^6.0}"
PHP_LSP_RS_VERSION="${PHP_LSP_RS_VERSION:-latest}"
PHPANTOM_VERSION="${PHPANTOM_VERSION:-latest}"

WITH_PSALM=0
CHECK_ONLY=0
for arg in "$@"; do
  case "${arg}" in
    --with-psalm) WITH_PSALM=1 ;;
    --check)      CHECK_ONLY=1 ;;
    -h|--help)    sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "unknown option: ${arg}" >&2; exit 1 ;;
  esac
done

log() { echo "[bench-install] $*"; }

# rust_target echoes the release-asset triple for this machine.
rust_target() {
  local arch os
  case "$(uname -m)" in
    arm64|aarch64) arch="aarch64" ;;
    x86_64|amd64)  arch="x86_64" ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; return 1 ;;
  esac
  case "$(uname -s)" in
    Darwin) os="apple-darwin" ;;
    Linux)  os="unknown-linux-gnu" ;;
    *) echo "unsupported OS: $(uname -s)" >&2; return 1 ;;
  esac
  echo "${arch}-${os}"
}

# install_release_tarball <repo> <version> <asset-prefix> <binary-name>
#
# Downloads a GitHub release tarball, verifies its .sha256 sidecar when the
# release publishes one, and drops the binary in bench/.tools/.
install_release_tarball() {
  local repo="$1" version="$2" prefix="$3" binary="$4"
  local target base url tmp
  target="$(rust_target)" || return 0
  base="${prefix}-${target}.tar.gz"
  if [[ "${version}" == "latest" ]]; then
    url="https://github.com/${repo}/releases/latest/download/${base}"
  else
    url="https://github.com/${repo}/releases/download/${version}/${base}"
  fi

  log "downloading ${binary} from ${url}"
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' RETURN
  if ! curl -fsSL --retry 3 -o "${tmp}/${base}" "${url}"; then
    log "  -> download failed; skipping ${binary}"
    return 0
  fi

  if curl -fsSL --retry 2 -o "${tmp}/${base}.sha256" "${url}.sha256" 2>/dev/null; then
    local want have
    want="$(tr -d '\r' < "${tmp}/${base}.sha256" | awk '{print $1}')"
    have="$(shasum -a 256 "${tmp}/${base}" | awk '{print $1}')"
    if [[ "${want}" != "${have}" ]]; then
      log "  -> CHECKSUM MISMATCH for ${base} (want ${want}, got ${have}); refusing to install"
      return 1
    fi
    log "  -> sha256 verified"
  else
    log "  -> no published checksum for this asset"
  fi

  tar -xzf "${tmp}/${base}" -C "${tmp}"
  local found
  found="$(find "${tmp}" -type f -name "${binary}" -perm -u+x | head -1)"
  if [[ -z "${found}" ]]; then
    found="$(find "${tmp}" -type f -name "${binary}" | head -1)"
  fi
  if [[ -z "${found}" ]]; then
    log "  -> ${binary} not found inside the tarball; skipping"
    return 0
  fi
  mv "${found}" "${TOOLS_DIR}/${binary}"
  chmod +x "${TOOLS_DIR}/${binary}"
}

report() {
  local name="$1" path="$2" version_cmd="${3:-}"
  if [[ -e "${path}" ]]; then
    local version=""
    if [[ -n "${version_cmd}" ]]; then
      version="$(eval "${version_cmd}" 2>/dev/null | head -1 || true)"
    fi
    printf '  %-14s installed  %s\n' "${name}" "${version}"
  else
    printf '  %-14s missing\n' "${name}"
  fi
}

check() {
  echo "bench tools in ${TOOLS_DIR}:"
  report "tusk-php" "${REPO_ROOT}/build/tusk-php" "'${REPO_ROOT}/build/tusk-php' --version"
  # intelephense has no --version flag; ask the package instead.
  report "intelephense" "${TOOLS_DIR}/intelephense/node_modules/.bin/intelephense" "node -p \"require('${TOOLS_DIR}/intelephense/node_modules/intelephense/package.json').version\""
  report "phpactor" "${TOOLS_DIR}/phpactor.phar" "php '${TOOLS_DIR}/phpactor.phar' --version"
  report "php-lsp-rs" "${TOOLS_DIR}/php-lsp" "'${TOOLS_DIR}/php-lsp' --version"
  report "phpantom" "${TOOLS_DIR}/phpantom_lsp" "'${TOOLS_DIR}/phpantom_lsp' --version"
  report "psalm" "${TOOLS_DIR}/psalm/vendor/bin/psalm-language-server" "php '${TOOLS_DIR}/psalm/vendor/bin/psalm' --version"
}

if [[ ${CHECK_ONLY} -eq 1 ]]; then
  check
  exit 0
fi

mkdir -p "${TOOLS_DIR}"

# --- tusk-php ---------------------------------------------------------------
if [[ ! -x "${REPO_ROOT}/build/tusk-php" ]]; then
  log "building tusk-php"
  make -C "${REPO_ROOT}" build
fi

# --- Intelephense (Node, free tier) -----------------------------------------
if command -v npm >/dev/null 2>&1; then
  log "installing intelephense@${INTELEPHENSE_VERSION}"
  mkdir -p "${TOOLS_DIR}/intelephense"
  npm install --silent --no-fund --no-audit \
    --prefix "${TOOLS_DIR}/intelephense" "intelephense@${INTELEPHENSE_VERSION}"
else
  log "npm not found — skipping intelephense"
fi

# --- Phpactor (PHP phar) ----------------------------------------------------
if [[ "${PHPACTOR_VERSION}" == "latest" ]]; then
  PHPACTOR_URL="https://github.com/phpactor/phpactor/releases/latest/download/phpactor.phar"
else
  PHPACTOR_URL="https://github.com/phpactor/phpactor/releases/download/${PHPACTOR_VERSION}/phpactor.phar"
fi
log "downloading phpactor from ${PHPACTOR_URL}"
curl -fsSL --retry 3 -o "${TOOLS_DIR}/phpactor.phar" "${PHPACTOR_URL}"
chmod +x "${TOOLS_DIR}/phpactor.phar"

# --- php-lsp (Rust, jorgsowa) -----------------------------------------------
install_release_tarball "jorgsowa/php-lsp" "${PHP_LSP_RS_VERSION}" "php-lsp" "php-lsp"

# --- PHPantom (Rust) --------------------------------------------------------
install_release_tarball "PHPantom-dev/phpantom_lsp" "${PHPANTOM_VERSION}" "phpantom_lsp" "phpantom_lsp"

# --- Psalm (optional) -------------------------------------------------------
if [[ ${WITH_PSALM} -eq 1 ]]; then
  if command -v composer >/dev/null 2>&1; then
    log "installing vimeo/psalm ${PSALM_CONSTRAINT}"
    mkdir -p "${TOOLS_DIR}/psalm"
    composer --quiet --working-dir="${TOOLS_DIR}/psalm" require "vimeo/psalm:${PSALM_CONSTRAINT}"
  else
    log "composer not found — skipping psalm"
  fi
fi

echo
check
echo
log "done. Run: go run ./bench/lspbench -list"
