# Deployment Guide

## System Requirements

### Server (Linux)
- **OS**: Linux 4.x+ with TUN/TAP support
- **Architectures**: amd64, arm64
- **RAM**: 64 MB minimum, 256 MB recommended
- **Disk**: 50 MB for binary + logs
- **Network**: Public IP or port forwarding for port 2222
- **Kernel**: `tun` module loaded (`lsmod | grep tun`)

### Client
- **Linux**: Kernel 3.x+ with TUN support
- **macOS**: 10.15+ (Catalina)
- **Windows**: 10+ with TAP driver installed
- **iOS**: 14+ (via NetworkExtension)
- **Android**: 7.0+ (API 24)

---

## Quick Start (Server)

```bash
# 1. Download binary
wget https://github.com/nnip7777/ssh-vpn/releases/download/v0.1.0/ssh-vpn-server-linux-amd64
chmod +x ssh-vpn-server-linux-amd64

# 2. Generate host key
./ssh-vpn-server-linux-amd64 -generate-key

# 3. Add authorized keys
echo "ssh-ed25519 AAAA... user@host" > authorized_keys

# 4. Create config
cat > server.yaml << 'EOF'
server:
  listen_addr: "0.0.0.0"
  listen_port: 2222
  max_clients: 100
  tun_name: "ssh-vpn0"
  tun_addr: "10.0.0.1"
  tun_netmask: "255.255.255.0"
  mtu: 1400
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
  compression: "lz4"
monitor:
  enabled: true
  listen_addr: "127.0.0.1"
  listen_port: 9090
EOF

# 5. Start server
sudo ./ssh-vpn-server-linux-amd64 -config server.yaml
```

---

## Quick Start (Client)

```bash
# 1. Download binary
wget https://github.com/nnip7777/ssh-vpn/releases/download/v0.1.0/ssh-vpn-client-linux-amd64
chmod +x ssh-vpn-client-linux-amd64

# 2. Create config
cat > client.yaml << 'EOF'
client:
  server_addr: "your-server.com"
  server_port: 2222
  username: "vpnuser"
  password: ""
  private_key_path: "~/.ssh/id_rsa"
  tun_name: "ssh-vpn0"
  tun_addr: "10.0.0.2"
  tun_netmask: "255.255.255.0"
  mtu: 1400
  auto_connect: true
channels:
  min_read: 2
  max_read: 8
  min_write: 1
  max_write: 4
  read_ratio: 0.8
  write_ratio: 0.2
security:
  compression: "lz4"
EOF

# 3. Start client
sudo ./ssh-vpn-client-linux-amd64 -config client.yaml
```

---

## Production Deployment

### Systemd Service (Server)

Create `/etc/systemd/system/ssh-vpn.service`:

```ini
[Unit]
Description=SSH VPN Server
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/ssh-vpn-server -config /etc/ssh-vpn/server.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable ssh-vpn
sudo systemctl start ssh-vpn
sudo systemctl status ssh-vpn
```

### Systemd Service (Client)

Create `/etc/systemd/system/ssh-vpn-client.service`:

```ini
[Unit]
Description=SSH VPN Client
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/ssh-vpn-client -config /etc/ssh-vpn/client.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

---

## Firewall Rules

### Server (iptables)

```bash
# Allow SSH VPN port
iptables -A INPUT -p tcp --dport 2222 -j ACCEPT

# Allow TUN traffic
iptables -A INPUT -i ssh-vpn0 -j ACCEPT
iptables -A OUTPUT -o ssh-vpn0 -j ACCEPT

# NAT for internet access through VPN
iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE

# Save rules
iptables-save > /etc/iptables.rules
```

### Server (nftables)

```nft
table inet filter {
    chain input {
        type filter hook input priority 0; policy accept;
        tcp dport 2222 accept
        iifname "ssh-vpn0" accept
    }
    chain forward {
        type filter hook forward priority 0; policy accept;
        iifname "ssh-vpn0" accept
        oifname "ssh-vpn0" accept
    }
}

table inet nat {
    chain postrouting {
        type nat hook postrouting priority 100;
        oifname "eth0" ip saddr 10.0.0.0/24 masquerade
    }
}
```

---

## Kernel Requirements

```bash
# Check TUN module
lsmod | grep tun

# Load if missing
modprobe tun

# Make persistent
echo "tun" >> /etc/modules-load.d/tun.conf

# Enable IP forwarding (for routing)
echo "net.ipv4.ip_forward = 1" >> /etc/sysctl.d/99-vpn.conf
sysctl -p /etc/sysctl.d/99-vpn.conf
```

---

## Generating SSH Keys

### Server Host Key
```bash
./ssh-vpn-server -generate-key
# Creates: host_key
```

### Client Key Pair
```bash
# Generate key
ssh-keygen -t ed25519 -f ~/.ssh/vpn_key -N ""

# Copy public key to server
ssh-copy-id -i ~/.ssh/vpn_key.pub user@server
```

---

## Monitoring

### Prometheus Metrics

When `monitor.enabled: true`, metrics available at `http://127.0.0.1:9090/metrics`:

```
ssh_vpn_clients_connected        # Current connected clients
ssh_vpn_channels_read_total      # Total read channels
ssh_vpn_channels_write_total     # Total write channels
ssh_vpn_bytes_sent_total         # Total bytes sent
ssh_vpn_bytes_recv_total         # Total bytes received
ssh_vpn_packets_sent_total       # Total packets sent
ssh_vpn_packets_recv_total       # Total packets received
ssh_vpn_channel_latency_seconds  # Channel latency
ssh_vpn_channel_packet_loss      # Channel packet loss ratio
```

### Grafana Dashboard

Import dashboard ID `XXXXX` (coming in v0.2.0).

---

## Verification

```bash
# Check server is listening
ss -tlnp | grep 2222

# Check TUN interface
ip addr show ssh-vpn0

# Test SSH connection
ssh -p 2222 user@server -o StrictHostKeyChecking=no

# Check metrics
curl http://localhost:9090/metrics
```
