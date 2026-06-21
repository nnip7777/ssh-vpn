# SSH VPN

[![Version](https://img.shields.io/badge/version-0.1.0-blue.svg)](https://github.com/nnip7777/ssh-vpn/releases)
[![Go](https://img.shields.io/badge/go-1.22+-00ADD8.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows%20%7C%20iOS%20%7C%20Android-lightgrey.svg)](#platform-support)

Multi-channel VPN with load balancing and fault tolerance over SSH.

```
┌─────────────────────────────────────────────────────────┐
│                    SSH VPN Architecture                  │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  Client                                                  │
│  ┌─────────┐    ┌──────────────────────────────────┐   │
│  │   TUN   │◄──►│   Channel Manager (Read/Write)    │   │
│  └─────────┘    │   Load Balancer + Fault Tolerance │   │
│                 └───────────────┬──────────────────┘   │
│                                 │ SSH (AES-256-GCM)     │
│  Server                         │                       │
│  ┌──────────────────────────────▼──────────────────┐   │
│  │   SSH Server → Channel Manager → TUN Interface  │   │
│  └─────────────────────────────────────────────────┘   │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Features

- **Multi-Channel**: Multiple SSH connections per client
- **Load Balancing**: Weighted round-robin across channels
- **Fault Tolerance**: Automatic failover on channel failure
- **Read/Write Split**: 80/20 bandwidth allocation (configurable)
- **Compression**: LZ4 compression for bandwidth savings
- **Cross-Platform**: Linux, macOS, Windows, iOS, Android
- **Encrypted**: AES-256-GCM via SSH protocol

## Quick Start

### Server (Linux)

```bash
# Download
wget https://github.com/nnip7777/ssh-vpn/releases/download/v0.1.0/ssh-vpn-server-linux-amd64
chmod +x ssh-vpn-server-linux-amd64

# Generate key and start
./ssh-vpn-server-linux-amd64 -generate-key
echo "ssh-ed25519 AAAA... user@host" > authorized_keys
sudo ./ssh-vpn-server-linux-amd64
```

### Client

```bash
# Download
wget https://github.com/nnip7777/ssh-vpn/releases/download/v0.1.0/ssh-vpn-client-linux-amd64
chmod +x ssh-vpn-client-linux-amd64

# Configure and start
cat > client.yaml << 'EOF'
client:
  server_addr: "your-server.com"
  server_port: 2222
  username: "vpnuser"
  private_key_path: "~/.ssh/id_rsa"
channels:
  min_read: 2
  max_read: 8
  read_ratio: 0.8
  write_ratio: 0.2
EOF
sudo ./ssh-vpn-client-linux-amd64 -config client.yaml
```

## Platform Support

| Platform | Architecture | Binary |
|----------|-------------|--------|
| Linux (server) | amd64, arm64 | `ssh-vpn-server-linux-*` |
| macOS (client) | amd64, arm64 | `ssh-vpn-client-macos-*` |
| Linux (client) | amd64, arm64 | `ssh-vpn-client-linux-*` |
| Windows (client) | amd64 | `ssh-vpn-client-windows-*.exe` |
| iOS (client) | arm64 | `SSHVPN.xcframework` |
| Android (client) | arm64 | `ssh-vpn.aar` |

## Building

```bash
# All platforms
./build_all.sh

# Individual
./build_ios.sh      # iOS framework
./build_android.sh  # Android AAR
```

## Configuration

See [docs/CONFIG.md](docs/CONFIG.md) for complete configuration reference.

### Server Config

```yaml
server:
  listen_port: 2222
  max_clients: 100
  tun_name: "ssh-vpn0"
  tun_addr: "10.0.0.1"
channels:
  min_read: 2
  max_read: 8
  read_ratio: 0.8
  write_ratio: 0.2
```

### Client Config

```yaml
client:
  server_addr: "your-server.com"
  server_port: 2222
  username: "vpnuser"
  private_key_path: "~/.ssh/id_rsa"
channels:
  min_read: 2
  max_read: 8
  read_ratio: 0.8
  write_ratio: 0.2
```

## Documentation

| Document | Description |
|----------|-------------|
| [Configuration](docs/CONFIG.md) | Complete config reference |
| [Deployment](docs/DEPLOYMENT.md) | Production deployment guide |
| [Architecture](docs/ARCHITECTURE.md) | System architecture |
| [Protocol](docs/PROTOCOL.md) | Wire protocol specification |
| [Mobile](docs/MOBILE.md) | iOS/Android integration |
| [Troubleshooting](docs/TROUBLESHOOTING.md) | Common issues and fixes |
| [Security](docs/SECURITY.md) | Security best practices |

## Interactive Configurator

Generate client config interactively:

```bash
# Build configurator
go build -o ssh-vpn-config ./cmd/configurator

# Run
./ssh-vpn-config
```

```
╔══════════════════════════════════════════╗
║       SSH VPN Client Configurator       ║
╚══════════════════════════════════════════╝

[1] Server address: your-server.com
[2] Server port: 2222
[3] Username: vpnuser
[4] Authentication: [1] Key [2] Password
[5] Key path: ~/.ssh/id_rsa
[6] Channel mode: [1] Home [2] Office [3] Mobile [4] Custom
```

## CLI Flags

### Server
```
ssh-vpn-server [flags]
  -config string      Config file path (default "server.yaml")
  -generate-key       Generate host key and exit
  -version            Show version
```

### Client
```
ssh-vpn-client [flags]
  -config string      Config file path (default "client.yaml")
  -version            Show version
```

## Architecture

- **Channels**: Read (80%) and Write (20%) channels with weighted round-robin
- **Health Check**: 5s intervals, 30s timeout
- **Failover**: Automatic channel replacement on failure
- **Compression**: LZ4 for bandwidth optimization

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for details.

## License

MIT License - see [LICENSE](LICENSE) for details.
