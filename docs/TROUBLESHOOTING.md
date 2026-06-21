# Troubleshooting

## Common Issues

### 1. "failed to create TUN interface"

**Cause**: TUN kernel module not loaded or no permissions.

**Fix**:
```bash
# Check if tun module is loaded
lsmod | grep tun

# Load it
sudo modprobe tun

# Make persistent
echo "tun" | sudo tee /etc/modules-load.d/tun.conf

# Ensure running as root
sudo ./ssh-vpn-server -config server.yaml
```

### 2. "failed to open /dev/net/tun: permission denied"

**Cause**: Not running as root or device permissions wrong.

**Fix**:
```bash
# Run as root
sudo ./ssh-vpn-server

# Or add user to correct group
sudo usermod -aG tun $USER
```

### 3. "connection refused" on client

**Cause**: Server not running or firewall blocking port.

**Fix**:
```bash
# Check server is running
ss -tlnp | grep 2222

# Check firewall
sudo iptables -L -n | grep 2222

# Test connectivity
nc -zv server-ip 2222
```

### 4. "authentication failed"

**Cause**: Wrong credentials or key not authorized.

**Fix**:
```bash
# Check authorized_keys
cat authorized_keys

# Ensure correct permissions
chmod 600 host_key
chmod 644 authorized_keys

# Test SSH directly
ssh -p 2222 user@server -v
```

### 5. "no read/write channels available"

**Cause**: All channels failed health check.

**Fix**:
```bash
# Check server logs for channel errors
journalctl -u ssh-vpn -f

# Increase channel limits in config
channels:
  min_read: 4
  max_read: 16

# Decrease timeout
channels:
  timeout: 15s
```

### 6. VPN connected but no internet access

**Cause**: IP forwarding disabled or NAT not configured.

**Fix**:
```bash
# Enable IP forwarding
echo "net.ipv4.ip_forward = 1" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p

# Add NAT rule
sudo iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o eth0 -j MASQUERADE
```

### 7. Slow throughput

**Cause**: MTU too large, compression issues, or channel count too low.

**Fix**:
```yaml
# Lower MTU
client:
  mtu: 1200

# Add more channels
channels:
  min_read: 4
  max_read: 16
  min_write: 2
  max_write: 8

# Enable compression
security:
  compression: "lz4"
```

### 8. Frequent disconnects

**Cause**: Network instability or timeout too short.

**Fix**:
```yaml
channels:
  health_check: 3s
  timeout: 15s

# Client auto-reconnect is enabled by default
client:
  auto_connect: true
```

---

## Debug Mode

### Enable Verbose Logging

Set environment variable before starting:

```bash
# Server
LOG_LEVEL=debug ./ssh-vpn-server -config server.yaml

# Client
LOG_LEVEL=debug ./ssh-vpn-client -config client.yaml
```

### Check Logs

```bash
# Systemd
journalctl -u ssh-vpn -f

# Direct output
./ssh-vpn-server 2>&1 | tee server.log
```

---

## Diagnostic Commands

```bash
# Check version
./ssh-vpn-server -version
./ssh-vpn-client -version

# Test SSH connection
ssh -p 2222 user@server -v

# Monitor channels in real-time
watch -n 1 'ss -tlnp | grep 2222'

# Check TUN interface
ip addr show ssh-vpn0
ip route show | grep ssh-vpn0

# Monitor bandwidth
iftop -i ssh-vpn0

# Check for errors in logs
grep -i error /var/log/syslog | grep ssh-vpn
```

---

## Performance Tuning

### High-Speed Network (>100 Mbps)
```yaml
channels:
  min_read: 4
  max_read: 16
  min_write: 2
  max_write: 8
  read_ratio: 0.8
  write_ratio: 0.2
server:
  mtu: 1500
```

### Mobile/Cellular (<10 Mbps)
```yaml
channels:
  min_read: 2
  max_read: 6
  min_write: 1
  max_write: 3
  health_check: 3s
  timeout: 15s
server:
  mtu: 1200
security:
  compression: "lz4"
```

### High Latency (>100ms)
```yaml
channels:
  health_check: 3s
  timeout: 20s
```

---

## Getting Help

1. Check this troubleshooting guide
2. Run with `-version` to verify build
3. Enable debug logging
4. Check GitHub Issues: https://github.com/nnip7777/ssh-vpn/issues
5. Provide logs and config when reporting issues
