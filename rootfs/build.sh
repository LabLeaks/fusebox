#!/usr/bin/env bash
# Build multi-arch rootfs tarballs for fusebox.
# Produces: fusebox-rootfs-arm64.tar.gz, fusebox-rootfs-amd64.tar.gz
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUT_DIR="${1:-$SCRIPT_DIR}"

for ARCH in arm64 amd64; do
    PLATFORM="linux/$ARCH"
    TAG="fusebox-rootfs:$ARCH"
    TARBALL="$OUT_DIR/fusebox-rootfs-$ARCH.tar.gz"

    echo "=== Building rootfs for $ARCH ==="
    docker buildx build \
        --platform "$PLATFORM" \
        --tag "$TAG" \
        --load \
        "$SCRIPT_DIR"

    echo "=== Exporting $TARBALL ==="
    CONTAINER=$(docker create --platform "$PLATFORM" "$TAG")
    docker export "$CONTAINER" | gzip > "$TARBALL"
    docker rm "$CONTAINER" > /dev/null

    echo "=== $TARBALL ready ($(du -h "$TARBALL" | cut -f1)) ==="
done

echo ""
echo "Done. Tarballs:"
ls -lh "$OUT_DIR"/fusebox-rootfs-*.tar.gz
