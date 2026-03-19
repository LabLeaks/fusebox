#!/bin/sh
set -e

REPO="lableaks/fusebox"
INSTALL_DIR="${HOME}/.local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64)        ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
esac

URL="https://github.com/${REPO}/releases/latest/download/work-${OS}-${ARCH}"
echo "Downloading work for ${OS}/${ARCH}..."
curl -sSL "$URL" -o /tmp/work-install
chmod +x /tmp/work-install
mkdir -p "$INSTALL_DIR"
mv /tmp/work-install "$INSTALL_DIR/work"
echo "Installed to $INSTALL_DIR/work"
echo ""
echo "Make sure $INSTALL_DIR is in your PATH, then run:"
echo "  work init user@your-server"
