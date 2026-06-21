#!/bin/bash

set -e

echo "=== Building SSH VPN for iOS ==="

# Check if gomobile is installed
if ! command -v gomobile &> /dev/null; then
    echo "Installing gomobile..."
    go install golang.org/x/mobile/cmd/gomobile@latest
    gomobile init
fi

echo "Building iOS framework..."
gomobile bind -v -target=ios -iosversion=14.0 \
    -o build/SSHVPN.xcframework \
    ./mobile/lib

echo ""
echo "=== iOS Build Complete ==="
echo "Framework: build/SSHVPN.xcframework"
echo ""
echo "To use in Xcode:"
echo "  1. Drag SSHVPN.xcframework into your project"
echo "  2. Import SSHVPN"
echo "  3. See examples/ios/ for usage"
