#!/bin/bash

set -e

echo "=== SSH VPN Build Script ==="
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed"
    exit 1
fi

echo "Go version: $(go version)"
echo ""

# Create build directory
mkdir -p build

# Build server (Linux only)
echo "Building server..."
GOOS=linux GOARCH=amd64 go build -o build/ssh-vpn-server-linux-amd64 ./cmd/server
GOOS=linux GOARCH=arm64 go build -o build/ssh-vpn-server-linux-arm64 ./cmd/server
echo "  ✓ Server built for Linux (amd64, arm64)"

# Build clients
echo ""
echo "Building clients..."

# macOS
echo "  macOS..."
GOOS=darwin GOARCH=amd64 go build -o build/ssh-vpn-client-macos-amd64 ./cmd/client
GOOS=darwin GOARCH=arm64 go build -o build/ssh-vpn-client-macos-arm64 ./cmd/client
echo "    ✓ macOS (amd64, arm64)"

# Linux
echo "  Linux..."
GOOS=linux GOARCH=amd64 go build -o build/ssh-vpn-client-linux-amd64 ./cmd/client
GOOS=linux GOARCH=arm64 go build -o build/ssh-vpn-client-linux-arm64 ./cmd/client
echo "    ✓ Linux (amd64, arm64)"

# Windows
echo "  Windows..."
GOOS=windows GOARCH=amd64 go build -o build/ssh-vpn-client-windows-amd64.exe ./cmd/client
echo "    ✓ Windows (amd64)"

echo ""
echo "=== Build Complete ==="
echo ""
echo "Binaries:"
ls -lh build/
echo ""
echo "Next steps:"
echo "  1. Generate host key: ./build/ssh-vpn-server-linux-amd64 -generate-key"
echo "  2. Edit server.yaml with your configuration"
echo "  3. Start server: sudo ./build/ssh-vpn-server-linux-amd64 -config server.yaml"
echo "  4. Edit client.yaml with server address"
echo "  5. Start client: sudo ./build/ssh-vpn-client-linux-amd64 -config client.yaml"
