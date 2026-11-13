#!/usr/bin/env bash
# field-baseline.sh: hold real codebases to a diagnostics baseline.
#
# Usage:
#   bash scripts/field-baseline.sh [--update] [name ...]
#
# For each codebase in testdata/field/manifest.json (or only the names given),
# the script checks out the pinned commit into testdata/field/_repos/<name>,
# installs its Composer dependencies from the pinned lock file, and runs
# `tusk-php check` over the listed paths.
#
# Without --update it compares the findings with testdata/field/<name>.baseline
# and fails when they differ: new findings are printed with their positions,
# and findings the baseline lists that are no longer reported are shown as a
# diff, so the baseline only ever shrinks on purpose. With --update it rewrites
# the baselines instead.
#
# Needs git, jq, composer (for codebases with a lock) and build/tusk-php.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
FIELD_DIR="${REPO_ROOT}/testdata/field"
MANIFEST="${FIELD_DIR}/manifest.json"
REPOS_DIR="${FIELD_DIR}/_repos"
BINARY="${TUSK_PHP:-${REPO_ROOT}/build/tusk-php}"

UPDATE=0
NAMES=()
for arg in "$@"; do
  case "${arg}" in
    --update) UPDATE=1 ;;
    -*) echo "unknown option: ${arg}" >&2; exit 2 ;;
    *) NAMES+=("${arg}") ;;
  esac
done

for tool in git jq; do
  command -v "${tool}" >/dev/null || { echo "ERROR: ${tool} is required" >&2; exit 2; }
done
[[ -x "${BINARY}" ]] || { echo "ERROR: ${BINARY} not found; run make build" >&2; exit 2; }

log() { echo "[field-baseline] $*"; }

# fetch checks out the pinned commit, shallow, reusing a checkout that is
# already there.
fetch() {
  local name="$1" repo="$2" sha="$3" dest="${REPOS_DIR}/$1"
  if [[ "$(git -C "${dest}" rev-parse HEAD 2>/dev/null || true)" == "${sha}" ]]; then
    return
  fi
  log "${name}: fetching ${sha:0:12}"
  rm -rf "${dest}"
  mkdir -p "${dest}"
  git -C "${dest}" init --quiet
  git -C "${dest}" remote add origin "${repo}"
  git -C "${dest}" fetch --quiet --depth 1 origin "${sha}"
  git -C "${dest}" -c advice.detachedHead=false checkout --quiet FETCH_HEAD
}

# install puts the lock file in place and installs from it, once per lock.
install() {
  local name="$1" lock="$2" root_version="$3" dest="${REPOS_DIR}/$1"
  [[ "${lock}" == "null" ]] && return
  if [[ "${lock}" != "composer.lock" ]]; then
    cp "${FIELD_DIR}/${lock}" "${dest}/composer.lock"
  fi
  local stamp
  stamp="$(git hash-object "${dest}/composer.lock")"
  if [[ -d "${dest}/vendor" && "$(cat "${dest}/vendor/.field-lock" 2>/dev/null || true)" == "${stamp}" ]]; then
    return
  fi
  command -v composer >/dev/null || { echo "ERROR: composer is required for ${name}" >&2; exit 2; }
  log "${name}: installing Composer dependencies"
  # A monorepo that replaces its own packages needs its version, which a
  # detached checkout doesn't have.
  [[ "${root_version}" == "null" ]] || export COMPOSER_ROOT_VERSION="${root_version}"
  composer install --working-dir "${dest}" --no-interaction --no-progress --prefer-dist \
    --no-scripts --no-plugins --ignore-platform-reqs --quiet
  unset COMPOSER_ROOT_VERSION
  echo "${stamp}" > "${dest}/vendor/.field-lock"
}

status=0
while IFS= read -r entry; do
  name="$(jq -r '.name' <<<"${entry}")"
  if [[ ${#NAMES[@]} -gt 0 && ! " ${NAMES[*]} " =~ " ${name} " ]]; then
    continue
  fi
  fetch "${name}" "$(jq -r '.repo' <<<"${entry}")" "$(jq -r '.sha' <<<"${entry}")"
  install "${name}" "$(jq -r '.lock' <<<"${entry}")" "$(jq -r '.rootVersion' <<<"${entry}")"

  args=(check --root "${REPOS_DIR}/${name}")
  while IFS= read -r pattern; do args+=(--exclude "${pattern}"); done < <(jq -r '.exclude[]' <<<"${entry}")
  baseline="${FIELD_DIR}/${name}.baseline"
  if [[ ${UPDATE} -eq 1 ]]; then
    log "${name}: writing ${baseline#"${REPO_ROOT}/"}"
    rc=0
    "${BINARY}" "${args[@]}" --write-baseline "${baseline}" $(jq -r '.paths[]' <<<"${entry}") >/dev/null || rc=$?
    [[ ${rc} -le 1 ]] || { echo "ERROR: tusk-php check failed for ${name}" >&2; exit 2; }
    continue
  fi

  log "${name}: checking against ${baseline#"${REPO_ROOT}/"}"
  current="$(mktemp)"
  touch "${baseline}"
  # Prints the findings the baseline doesn't list, with positions.
  rc=0
  "${BINARY}" "${args[@]}" --baseline "${baseline}" --write-baseline "${current}" $(jq -r '.paths[]' <<<"${entry}") || rc=$?
  [[ ${rc} -le 1 ]] || { echo "ERROR: tusk-php check failed for ${name}" >&2; exit 2; }
  if ! diff -u --label "${name}.baseline" --label "${name} now" "${baseline}" "${current}" >"${current}.diff"; then
    status=1
    gone="$(grep -c '^-[^-]' "${current}.diff" || true)"
    added="$(grep -c '^+[^+]' "${current}.diff" || true)"
    echo "::error title=${name} diagnostics changed::${added} new, ${gone} no longer reported. If that's intended, run 'make field-baseline-update NAME=${name}' and commit the baseline."
    if [[ "${gone}" -gt 0 ]]; then
      echo "Findings the baseline lists that are no longer reported:"
      grep '^-[^-]' "${current}.diff" || true
    fi
  fi
  rm -f "${current}" "${current}.diff"
done < <(jq -c '.[]' "${MANIFEST}")

exit "${status}"
