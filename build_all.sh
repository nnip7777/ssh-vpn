#!/bin/bash

set -e

echo "=== Building SSH VPN ==="

# Check Go
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed"
    exit 1
fi

echo "Go version: $(go version)"

# Create build directory
mkdir -p build

echo ""
echo "--- Building configurator ---"
GOOS=linux GOARCH=amd64 go build -o build/ssh-vpn-config-linux-amd64 ./cmd/configurator
GOOS=linux GOARCH=arm64 go build -o build/ssh-vpn-config-linux-arm64 ./cmd/configurator
GOOS=darwin GOARCH=amd64 go build -o build/ssh-vpn-config-macos-amd64 ./cmd/configurator
GOOS=darwin GOARCH=arm64 go build -o build/ssh-vpn-config-macos-arm64 ./cmd/configurator
GOOS=windows GOARCH=amd64 go build -o build/ssh-vpn-config-windows-amd64.exe ./cmd/configurator
echo "  ✓ Configurator built"

echo ""
echo "--- Building Linux server ---"
GOOS=linux GOARCH=amd64 go build -o build/ssh-vpn-server-linux-amd64 ./cmd/server
GOOS=linux GOARCH=arm64 go build -o build/ssh-vpn-server-linux-arm64 ./cmd/server
echo "  ✓ Server built"

echo ""
echo "--- Building desktop clients ---"
GOOS=darwin GOARCH=amd64 go build -o build/ssh-vpn-client-macos-amd64 ./cmd/client
GOOS=darwin GOARCH=arm64 go build -o build/ssh-vpn-client-macos-arm64 ./cmd/client
GOOS=linux GOARCH=amd64 go build -o build/ssh-vpn-client-linux-amd64 ./cmd/client
GOOS=linux GOARCH=arm64 go build -o build/ssh-vpn-client-linux-arm64 ./cmd/client
GOOS=windows GOARCH=amd64 go build -o build/ssh-vpn-client-windows-amd64.exe ./cmd/client
echo "  ✓ Desktop clients built"

# Mobile builds (optional, requires gomobile)
if command -v gomobile &> /dev/null; then
    echo ""
    echo "--- Building for iOS ---"
    gomobile bind -v -target=ios -iosversion=14.0 \
        -o build/SSHVPN.xcframework \
        ./mobile/lib 2>&1 && echo "  ✓ iOS framework built" || echo "  ⚠ iOS build skipped"

    echo ""
    echo "--- Building for Android ---"
    gomobile bind -v -target=android \
        -o build/ssh-vpn.aar \
        ./mobile/lib 2>&1 && echo "  ✓ Android AAR built" || echo "  ⚠ Android build skipped"
else
    echo ""
    echo "--- Mobile builds skipped (gomobile not installed) ---"
    echo "  Install: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init"
fi

echo ""
echo "=== Build Complete ==="
echo ""
echo "Binaries:"
ls -lh build/ 2>/dev/null
echo ""
echo "=== Usage ==="
echo ""
echo "Configurator:"
echo "  ./build/ssh-vpn-config-linux-amd64"
echo ""
echo "Server (Linux):"
echo "  sudo ./build/ssh-vpn-server-linux-amd64 -config server.yaml"
echo ""
echo "Client:"
echo "  sudo ./build/ssh-vpn-client-linux-amd64 -config client.yaml"
