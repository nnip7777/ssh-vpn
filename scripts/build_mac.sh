#!/bin/bash
set -e

echo "=== SSH VPN Client Build for macOS ==="

# Check Go
if ! command -v go &> /dev/null; then
    echo "Installing Go..."
    brew install go
fi

echo "Go: $(go version)"

# Get source
SRC=~/vpn-ssh-src
if [ ! -d "$SRC" ]; then
    echo "Downloading source..."
    scp -r root@72.56.246.125:/root/ssh-vpn "$SRC"
fi

# Build
echo "Building..."
cd "$SRC"
go build -o ~/vpn-ssh-client/ssh-vpn-client ./cmd/client

echo ""
echo "=== Done ==="
echo "Run: sudo ~/vpn-ssh-client/ssh-vpn-client -config ~/vpn-ssh-client/client.yaml"
