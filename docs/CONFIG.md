# Configuration Reference

Complete reference for all configuration options.

## Server Configuration (`server.yaml`)

```yaml
server:
  listen_addr: "0.0.0.0"      # Bind address
  listen_port: 2222             # SSH port
  max_clients: 100              # Max concurrent clients
  tun_name: "ssh-vpn0"         # TUN interface name
  tun_addr: "10.0.0.1"         # Server TUN IP
  tun_netmask: "255.255.255.0" # TUN netmask
  mtu: 1400                     # Maximum transmission unit

channels:
  min_read: 2                   # Min read channels per client
  max_read: 8                   # Max read channels per client
  min_write: 1                  # Min write channels per client
  max_write: 4                  # Max write channels per client
  read_ratio: 0.8               # Bandwidth ratio for reads (80%)
  write_ratio: 0.2              # Bandwidth ratio for writes (20%)
  health_check: 5s              # Health check interval
  timeout: 30s                  # Channel timeout

security:
  encryption: "aes256-gcm"      # Encryption algorithm
  compression: "lz4"            # Compression (lz4 or none)

monitor:
  enabled: true                 # Enable monitoring endpoint
  listen_addr: "127.0.0.1"     # Monitor bind address
  listen_port: 9090             # Monitor port
```

## Client Configuration (`client.yaml`)

```yaml
client:
  server_addr: "your-server.com"  # Server hostname/IP
  server_port: 2222                # Server SSH port
  username: "vpnuser"              # SSH username
  password: ""                     # SSH password (empty = key auth)
  private_key_path: "~/.ssh/id_rsa" # SSH private key
  tun_name: "ssh-vpn0"            # TUN interface name
  tun_addr: "10.0.0.2"            # Client TUN IP
  tun_netmask: "255.255.255.0"    # TUN netmask
  mtu: 1400                        # Maximum transmission unit
  auto_connect: true               # Connect on start

channels:
  min_read: 2
  max_read: 8
  min_write: 1
  max_write: 4
  read_ratio: 0.8
  write_ratio: 0.2
  health_check: 5s
  timeout: 30s

security:
  encryption: "aes256-gcm"
  compression: "lz4"
```

---

## Field Reference

### Server Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `server.listen_addr` | string | `0.0.0.0` | IP address to bind. Use `127.0.0.1` for local-only. |
| `server.listen_port` | int | `2222` | SSH port for incoming connections. |
| `server.max_clients` | int | `100` | Maximum concurrent client connections. |
| `server.tun_name` | string | `ssh-vpn0` | Name of the TUN network interface. |
| `server.tun_addr` | string | `10.0.0.1` | IP address assigned to server TUN. Must be in same subnet as client. |
| `server.tun_netmask` | string | `255.255.255.0` | Subnet mask for TUN interface. |
| `server.mtu` | int | `1400` | Maximum transmission unit. Range: 576-1500. Lower = better for slow links. |

### Client Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `client.server_addr` | string | `localhost` | Server hostname or IP to connect to. |
| `client.server_port` | int | `2222` | Server SSH port. Must match server config. |
| `client.username` | string | `""` | SSH username for authentication. |
| `client.password` | string | `""` | SSH password. Leave empty for key-based auth. |
| `client.private_key_path` | string | `~/.ssh/id_rsa` | Path to SSH private key file. |
| `client.tun_name` | string | `ssh-vpn0` | Client TUN interface name. |
| `client.tun_addr` | string | `10.0.0.2` | Client TUN IP. Must be different from server. |
| `client.tun_netmask` | string | `255.255.255.0` | Subnet mask. Must match server. |
| `client.mtu` | int | `1400` | Must match server MTU. |
| `client.auto_connect` | bool | `true` | Automatically connect on client start. |

### Channel Fields

| Field | Type | Default | Range | Description |
|-------|------|---------|-------|-------------|
| `channels.min_read` | int | `2` | 1-32 | Minimum read channels. More = better redundancy. |
| `channels.max_read` | int | `8` | 1-32 | Maximum read channels. Created on high load. |
| `channels.min_write` | int | `1` | 1-16 | Minimum write channels. |
| `channels.max_write` | int | `4` | 1-16 | Maximum write channels. |
| `channels.read_ratio` | float | `0.8` | 0.0-1.0 | Bandwidth allocated to read channels. |
| `channels.write_ratio` | float | `0.2` | 0.0-1.0 | Bandwidth allocated to write channels. Must satisfy: `read_ratio + write_ratio = 1.0`. |
| `channels.health_check` | duration | `5s` | 1s-60s | How often to check channel health. |
| `channels.timeout` | duration | `30s` | 5s-300s | Channel considered dead after this time without activity. |

### Security Fields

| Field | Type | Default | Options | Description |
|-------|------|---------|---------|-------------|
| `security.encryption` | string | `aes256-gcm` | `aes256-gcm` | Encryption algorithm. SSH handles this. |
| `security.compression` | string | `lz4` | `lz4`, `none` | Compression algorithm. `lz4` reduces bandwidth. |

### Monitor Fields (Server Only)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `monitor.enabled` | bool | `true` | Enable Prometheus metrics endpoint. |
| `monitor.listen_addr` | string | `127.0.0.1` | Metrics bind address. |
| `monitor.listen_port` | int | `9090` | Metrics port. Access at `http://addr:port/metrics`. |

---

## CLI Flags

### Server

```
ssh-vpn-server [flags]

  -config string      Path to config file (default "server.yaml")
  -generate-key       Generate SSH host key and exit
  -version            Show version and exit
```

### Client

```
ssh-vpn-client [flags]

  -config string      Path to config file (default "client.yaml")
  -version            Show version and exit
```

### Configurator

```
ssh-vpn-config [flags]

  -output string      Output config file path (default "client.yaml")
  -preset string      Use a preset: home, office, mobile, custom
```

---

## Presets

### Home (fast internet)
```yaml
channels:
  min_read: 4
  max_read: 8
  min_write: 2
  max_write: 4
  read_ratio: 0.8
  write_ratio: 0.2
security:
  compression: "lz4"
```

### Office (corporate network)
```yaml
channels:
  min_read: 2
  max_read: 4
  min_write: 1
  max_write: 2
  read_ratio: 0.7
  write_ratio: 0.3
security:
  compression: "none"
```

### Mobile (cellular, high latency)
```yaml
channels:
  min_read: 2
  max_read: 6
  min_write: 1
  max_write: 3
  read_ratio: 0.85
  write_ratio: 0.15
  health_check: 3s
  timeout: 15s
security:
  compression: "lz4"
```

---

## Network Topology

```
Server TUN: 10.0.0.1/24
Client 1:   10.0.0.2/24
Client 2:   10.0.0.3/24
Client 3:   10.0.0.4/24
```

Each client gets a unique IP in the TUN subnet. Server routes traffic between clients.

---

## Environment Variables

All config values can be overridden by environment variables:

| Env Variable | Config Field | Example |
|-------------|--------------|---------|
| `SSHVPN_SERVER_ADDR` | `client.server_addr` | `SSHVPN_SERVER_ADDR=1.2.3.4` |
| `SSHVPN_SERVER_PORT` | `client.server_port` | `SSHVPN_SERVER_PORT=2222` |
| `SSHVPN_USERNAME` | `client.username` | `SSHVPN_USERNAME=admin` |
| `SSHVPN_PASSWORD` | `client.password` | `SSHVPN_PASSWORD=secret` |
| `SSHVPN_CONFIG` | config file path | `SSHVPN_CONFIG=/etc/ssh-vpn/client.yaml` |
