#!/usr/bin/env bash
set -euo pipefail
VERSION="${VERSION:-0.9.0}"
OUTPUT_DIR="./build"
rm -rf "${OUTPUT_DIR}" && mkdir -p "${OUTPUT_DIR}"

# Helper: print the lowercase sha256 hex digest of a file.
sha256_of() {
    local file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | awk '{print $1}'
    else
        shasum -a 256 "$file" | awk '{print $1}'
    fi
}

# Accumulate manifest artifact entries as a newline-separated list of JSON
# objects (one per line); we join them with commas when writing the file.
manifest_entries=()

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
    IFS='/' read -r GOOS GOARCH <<< "$target"
    ext=""; [ "$GOOS" = "windows" ] && ext=".exe"
    out="${OUTPUT_DIR}/tusk-php-${GOOS}-${GOARCH}${ext}"
    echo "Building tusk-php ${GOOS}/${GOARCH}..."
    GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o "${out}" ./cmd/tusk-php/

    # Collect manifest entry for this artifact.
    filename="tusk-php-${GOOS}-${GOARCH}${ext}"
    digest="$(sha256_of "${out}")"
    manifest_entries+=("  { \"os\": \"${GOOS}\", \"arch\": \"${GOARCH}\", \"file\": \"${filename}\", \"sha256\": \"${digest}\" }")
done
echo "Build complete. Binaries in: ${OUTPUT_DIR}/"

# Build tusk-php for wasip1/wasm (browser / vscode.dev target).
wasm_out="${OUTPUT_DIR}/tusk-php.wasm"
echo "Building tusk-php wasip1/wasm..."
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o "${wasm_out}" ./cmd/tusk-php/
wasm_digest="$(sha256_of "${wasm_out}")"
manifest_entries+=("  { \"os\": \"wasi\", \"arch\": \"wasm\", \"file\": \"tusk-php.wasm\", \"sha256\": \"${wasm_digest}\" }")

ls -lh "${OUTPUT_DIR}/"

# SHA-256 checksums for the release artifacts. The editor extensions
# (Tusk-PHP/vscode, Tusk-PHP/zed) pin against these sums.
(
    cd "${OUTPUT_DIR}"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum tusk-* > checksums.txt
    else
        shasum -a 256 tusk-* > checksums.txt
    fi
)
echo "Checksums written to ${OUTPUT_DIR}/checksums.txt"
cat "${OUTPUT_DIR}/checksums.txt"

# Write manifest.json — join entries with commas, no trailing comma.
{
    printf '{\n'
    printf '  "version": "%s",\n' "${VERSION}"
    printf '  "artifacts": [\n'
    for i in "${!manifest_entries[@]}"; do
        if (( i < ${#manifest_entries[@]} - 1 )); then
            printf '%s,\n' "${manifest_entries[$i]}"
        else
            printf '%s\n' "${manifest_entries[$i]}"
        fi
    done
    printf '  ]\n'
    printf '}\n'
} > "${OUTPUT_DIR}/manifest.json"

echo "Manifest written to ${OUTPUT_DIR}/manifest.json"
cat "${OUTPUT_DIR}/manifest.json"

# Validate the manifest is well-formed JSON.
if command -v python3 >/dev/null 2>&1; then
    python3 -m json.tool "${OUTPUT_DIR}/manifest.json" >/dev/null && echo "manifest.json is valid JSON (python3)"
elif command -v jq >/dev/null 2>&1; then
    jq . "${OUTPUT_DIR}/manifest.json" >/dev/null && echo "manifest.json is valid JSON (jq)"
fi
