#!/bin/bash

set -e

APP_NAME="remoter"
BUILD_DIR="build"

mkdir -p $BUILD_DIR

# Build for Linux amd64
GOOS=linux GOARCH=amd64 go build -o $BUILD_DIR/$APP_NAME .

echo "Build complete: $BUILD_DIR/$APP_NAME"
