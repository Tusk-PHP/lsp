#!/usr/bin/env bash
set -euo pipefail
VERSION="${VERSION:-0.5.0}"
OUTPUT_DIR="./build"
rm -rf "${OUTPUT_DIR}" && mkdir -p "${OUTPUT_DIR}"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
    IFS='/' read -r GOOS GOARCH <<< "$target"
    ext=""; [ "$GOOS" = "windows" ] && ext=".exe"
    for binary in tusk-php tusk-mcp; do
        out="${OUTPUT_DIR}/${binary}-${GOOS}-${GOARCH}${ext}"
        echo "Building ${binary} ${GOOS}/${GOARCH}..."
        GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o "${out}" ./cmd/${binary}/
    done
done
echo "Build complete. Binaries in: ${OUTPUT_DIR}/"
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
