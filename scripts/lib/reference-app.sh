#!/usr/bin/env bash
# reference-app.sh: helpers shared by the scripts that fetch a reference app
# (scripts/fetch-*.sh). Source it; it defines functions and nothing else.
#
# A reference app is a real project for one framework, fetched at a pinned
# version into testdata/<name>/ (gitignored) for the benchmark and the field
# baselines. See testdata/field/README.md.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOCKS_DIR="${REPO_ROOT}/testdata/field/locks"

# need fails unless every tool named is on PATH.
need() {
  local tool
  for tool in "$@"; do
    command -v "${tool}" >/dev/null || { echo "ERROR: ${tool} is required" >&2; exit 2; }
  done
}

# git_checkout <repo> <sha> <dest> checks out one commit, shallow, reusing a
# checkout that is already at it.
git_checkout() {
  local repo="$1" sha="$2" dest="$3"
  if [[ "$(git -C "${dest}" rev-parse HEAD 2>/dev/null || true)" == "${sha}" ]]; then
    return
  fi
  log "fetching ${sha:0:12}"
  rm -rf "${dest}"
  mkdir -p "${dest}"
  git -C "${dest}" init --quiet
  git -C "${dest}" remote add origin "${repo}"
  git -C "${dest}" fetch --quiet --depth 1 origin "${sha}"
  git -C "${dest}" -c advice.detachedHead=false checkout --quiet FETCH_HEAD
}

# download <url> <sha256> <file> downloads a file once and refuses it when its
# checksum isn't the pinned one.
download() {
  local url="$1" sum="$2" file="$3"
  if [[ -f "${file}" ]] && echo "${sum}  ${file}" | shasum -a 256 -c --status; then
    return
  fi
  log "downloading ${url##*/}"
  curl -fsSL --retry 3 -o "${file}.part" "${url}"
  if ! echo "${sum}  ${file}.part" | shasum -a 256 -c --status; then
    rm -f "${file}.part"
    echo "ERROR: checksum mismatch for ${url}" >&2
    exit 1
  fi
  mv "${file}.part" "${file}"
}

# composer_install <dir> [composer args...] installs from the lock in <dir>
# once per lock: a vendor/ installed from the same lock is reused. Platform
# requirements are ignored because only the source is read, never run.
composer_install() {
  local dir="$1"
  shift
  local stamp
  stamp="$(git hash-object "${dir}/composer.lock")"
  if [[ -d "${dir}/vendor" && "$(cat "${dir}/vendor/.fetch-lock" 2>/dev/null || true)" == "${stamp}" ]]; then
    log "dependencies up to date"
    return
  fi
  log "installing Composer dependencies"
  composer install --working-dir "${dir}" --no-interaction --no-progress --prefer-dist \
    --ignore-platform-reqs --quiet "$@"
  echo "${stamp}" > "${dir}/vendor/.fetch-lock"
}
