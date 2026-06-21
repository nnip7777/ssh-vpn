#!/bin/bash

set -e

echo "=== Building SSH VPN for Android ==="

# Check if gomobile is installed
if ! command -v gomobile &> /dev/null; then
    echo "Installing gomobile..."
    go install golang.org/x/mobile/cmd/gomobile@latest
    gomobile init
fi

# Check ANDROID_HOME
if [ -z "$ANDROID_HOME" ]; then
    echo "Warning: ANDROID_HOME not set"
fi

echo "Building Android AAR..."
gomobile bind -v -target=android \
    -o build/ssh-vpn.aar \
    ./mobile/lib

echo ""
echo "=== Android Build Complete ==="
echo "AAR: build/ssh-vpn.aar"
echo ""
echo "To use in Android Studio:"
echo "  1. Copy ssh-vpn.aar to app/libs/"
echo "  2. Add implementation(files('libs/ssh-vpn.aar')) to build.gradle"
echo "  3. See examples/android/ for usage"
