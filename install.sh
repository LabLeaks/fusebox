#!/bin/sh
set -e

REPO="LabLeaks/fusebox"
INSTALL_DIR="${HOME}/.local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64)        ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
esac

ASSET="fusebox-${OS}-${ARCH}"

# Use gh CLI for private repos, fall back to curl for public
if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    echo "Downloading ${ASSET} via gh..."
    gh release download --repo "$REPO" --pattern "$ASSET" --dir /tmp --clobber
    mv "/tmp/${ASSET}" /tmp/fusebox-install
else
    URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
    echo "Downloading ${ASSET}..."
    curl -fsSL "$URL" -o /tmp/fusebox-install
fi

chmod +x /tmp/fusebox-install
mkdir -p "$INSTALL_DIR"
mv /tmp/fusebox-install "$INSTALL_DIR/fusebox"
echo "Installed to $INSTALL_DIR/fusebox"
echo ""
echo "Make sure $INSTALL_DIR is in your PATH, then run:"
echo "  fusebox init user@your-server"
