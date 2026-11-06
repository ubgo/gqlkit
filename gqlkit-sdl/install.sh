#!/bin/sh
set -e

REPO="ubgo/gqlkit"
BINARY="gqlkit-sdl"
TAG_PREFIX="gqlkit-sdl@"
# Overrides: VERSION=v0.14.0 pins a release (default: latest),
# INSTALL_DIR=$HOME/.local/bin installs without sudo (default: /usr/local/bin).
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-}"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" && exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS" && exit 1 ;;
esac

# Use the pinned VERSION, or find the latest release matching this tool's tag prefix
if [ -n "$VERSION" ]; then
  TAG="${TAG_PREFIX}v${VERSION#v}"
else
  TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases" \
    | grep -o "\"tag_name\": *\"${TAG_PREFIX}v[^\"]*\"" \
    | head -1 \
    | cut -d'"' -f4)
fi

if [ -z "$TAG" ]; then
  echo "Error: No release found for ${BINARY}" && exit 1
fi

ENCODED_TAG="$TAG"
ASSET="${BINARY}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${ENCODED_TAG}/${ASSET}"

echo "Downloading ${BINARY} (${TAG})..."
tmpdir=$(mktemp -d)
if ! curl -fsSL "$URL" -o "$tmpdir/$ASSET"; then
  echo "Error: could not download $URL (does release ${TAG} exist for ${OS}/${ARCH}?)" && rm -rf "$tmpdir" && exit 1
fi
tar xzf "$tmpdir/$ASSET" -C "$tmpdir"

echo "Installing to ${INSTALL_DIR}/${BINARY}..."
mkdir -p "$INSTALL_DIR" 2>/dev/null || true
if [ -w "$INSTALL_DIR" ]; then
  mv "$tmpdir/$BINARY" "$INSTALL_DIR/$BINARY"
else
  sudo mv "$tmpdir/$BINARY" "$INSTALL_DIR/$BINARY"
fi
rm -rf "$tmpdir"

echo "Done! Run 'gqlkit-sdl version' to verify."
