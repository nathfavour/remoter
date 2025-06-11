#!/bin/bash

set -e

APP_NAME="remoter"
BUILD_DIR="build"
INSTALL_PATH="/usr/local/bin/$APP_NAME"

if [ ! -f "$BUILD_DIR/$APP_NAME" ]; then
    echo "Binary not found. Run ./build.sh first."
    exit 1
fi

sudo cp "$BUILD_DIR/$APP_NAME" "$INSTALL_PATH"
sudo chmod +x "$INSTALL_PATH"

echo "$APP_NAME installed to $INSTALL_PATH"
