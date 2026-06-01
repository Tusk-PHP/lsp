#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat <<'EOF'
Usage: scripts/set-version.sh <version>

Updates the project-owned release version fields across the repository.
(Editor extension versions are managed in their own repos: Tusk-PHP/vscode,
Tusk-PHP/zed.)

Examples:
  scripts/set-version.sh 0.2.1
  scripts/set-version.sh 0.3.0-beta.1
EOF
}

if [[ $# -lt 1 ]]; then
    usage
    exit 1
fi

VERSION=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            usage
            exit 0
            ;;
        *)
            if [[ -n "$VERSION" ]]; then
                echo "Unexpected argument: $1" >&2
                usage
                exit 1
            fi
            VERSION="$1"
            shift
            ;;
    esac
done

if [[ -z "$VERSION" ]]; then
    echo "Missing version argument." >&2
    usage
    exit 1
fi

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]]; then
    echo "Invalid version: $VERSION" >&2
    echo "Expected semantic version like 0.2.1 or 0.3.0-beta.1" >&2
    exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export VERSION

replace_in_file() {
    local file="$1"
    local expression="$2"
    perl -0pi -e "$expression" "$file"
}

echo "Setting version to $VERSION"

replace_in_file "Makefile" 's/^VERSION \?= .+$/VERSION ?= $ENV{VERSION}/m'
replace_in_file "scripts/build.sh" 's/^VERSION="\$\{VERSION:-[^}]+\}"$/VERSION="\${VERSION:-$ENV{VERSION}}"/m'
replace_in_file "scripts/install.sh" 's/^VERSION="\$\{VERSION:-[^}]+\}"$/VERSION="\${VERSION:-$ENV{VERSION}}"/m'
replace_in_file "cmd/tusk-php/main.go" 's/^(\s*version\s*=\s*")[^"]+(")/$1$ENV{VERSION}$2/m'
replace_in_file "internal/lsp/server.go" 's/^(const ServerVersion = ")[^"]+(")/$1$ENV{VERSION}$2/m'
replace_in_file "CONTRIBUTING.md" 's/(Pushing a semver tag \(e\.g\., `v)[^`]+(`\) triggers:)/$1$ENV{VERSION}$2/'

echo "Updated version references:"
printf '  %s\n' \
    "Makefile" \
    "scripts/build.sh" \
    "scripts/install.sh" \
    "cmd/tusk-php/main.go" \
    "internal/lsp/server.go" \
    "CONTRIBUTING.md"
