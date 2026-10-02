#!/usr/bin/env bash
set -e

REPO="staticmiro/heed"

echo "Installing heed..."

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ARCH="arm64"
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

if [ "$OS" != "linux" ] && [ "$OS" != "darwin" ]; then
    echo "Unsupported OS: $OS"
    exit 1
fi

BINARY_URL="https://github.com/$REPO/releases/latest/download/heed-$OS-$ARCH"

echo "Downloading heed for $OS/$ARCH..."
if ! curl -f -sSL -o /tmp/heed "$BINARY_URL"; then
    echo "Error: Failed to download heed binary from $BINARY_URL"
    echo "Please ensure that a GitHub release with assets exists at https://github.com/$REPO/releases"
    exit 1
fi
chmod +x /tmp/heed

echo "Installing to /usr/local/bin/heed..."
sudo mv /tmp/heed /usr/local/bin/heed

echo "Successfully installed heed!"
echo "Run 'heed -help' to get started."
