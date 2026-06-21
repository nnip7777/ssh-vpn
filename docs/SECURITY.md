# Security Guide

## Encryption

All traffic is encrypted via SSH (AES-256-GCM). The SSH protocol provides:

- **Confidentiality**: Data encrypted in transit
- **Integrity**: SHA-256 HMAC verification
- **Forward Secrecy**: Curve25519 key exchange

No additional encryption layer is needed on top of SSH.

---

## Authentication

### Public Key (Recommended)

Most secure method. Server verifies client's private key.

```bash
# Generate client key
ssh-keygen -t ed25519 -f ~/.ssh/vpn_key -N ""

# Add public key to server
echo "ssh-ed25519 AAAA... user@host" >> authorized_keys

# Client config
client:
  username: "vpnuser"
  private_key_path: "~/.ssh/vpn_key"
```

### Password Authentication

Simpler but less secure. Use strong passwords.

```yaml
client:
  username: "vpnuser"
  password: "your-strong-password-here"
```

**Warning**: Password auth is susceptible to brute-force attacks. Use fail2ban:

```bash
# Install fail2ban
apt install fail2ban

# Configure
cat > /etc/fail2ban/jail.local << 'EOF'
[sshd]
enabled = true
port = 2222
filter = sshd
logpath = /var/log/auth.log
maxretry = 3
bantime = 3600
EOF
```

### Certificate-Based (Enterprise)

For large deployments, use SSH certificates:

```bash
# Create CA
ssh-keygen -t ed25519 -f ca_key

# Sign server key
ssh-keygen -s ca_key -I server -h host_key.pub

# Sign client key
ssh-keygen -s ca_key -I user -n vpnuser client_key.pub
```

---

## Hardening Recommendations

### Server

1. **Use key-only authentication** (disable password):
   ```yaml
   # In server config, don't implement password callback
   ```

2. **Limit max auth tries**:
   ```yaml
   server:
     max_clients: 50  # Reduce from 100
   ```

3. **Use dedicated user**:
   ```bash
   useradd -r -s /bin/false vpnuser
   ```

4. **Restrict listen address**:
   ```yaml
   server:
     listen_addr: "10.0.0.1"  # Only listen on VPN subnet
   ```

5. **Monitor connections**:
   ```yaml
   monitor:
     enabled: true
     listen_addr: "127.0.0.1"  # Localhost only
   ```

### Network

1. **Firewall**: Only allow port 2222
2. **IP forwarding**: Enable only for VPN subnet
3. **NAT**: Restrict to VPN traffic

```bash
# Restrict forwarding
iptables -A FORWARD -s 10.0.0.0/24 -o eth0 -j ACCEPT
iptables -A FORWARD -i eth0 -d 10.0.0.0/24 -j ACCEPT
iptables -A FORWARD -j DROP
```

### Client

1. **Use key authentication** (no passwords in config)
2. **Protect private key**:
   ```bash
   chmod 600 ~/.ssh/vpn_key
   ```
3. **Don't run as root** (use capabilities instead):
   ```bash
   sudo setcap cap_net_admin+ep ./ssh-vpn-client
   ./ssh-vpn-client -config client.yaml
   ```

---

## Known Limitations

1. **No perfect forward secrecy per channel**: All channels share the same SSH session. Compromising the session key compromises all channels.

2. **No channel-level authentication**: Channels are multiplexed within an authenticated session. A compromised session gives full channel access.

3. **Single authentication method**: Cannot combine public key + password for multi-factor.

4. **No revocation mechanism**: To revoke access, remove from `authorized_keys` and restart server.

5. **TUN/TAP requires root**: On Linux, TUN device access requires root or CAP_NET_ADMIN.

---

## Audit Trail

Server logs all connection events:

```
2024-01-15T10:30:00Z INFO  client connected addr=1.2.3.4:54321 user=vpnuser
2024-01-15T10:30:01Z INFO  new channel opened client=1.2.3.4:54321 id=1 type=vpn-read
2024-01-15T10:30:01Z INFO  new channel opened client=1.2.3.4:54321 id=2 type=vpn-write
2024-01-15T10:45:00Z INFO  client disconnected addr=1.2.3.4:54321
```

Forward logs to SIEM:

```bash
# Rsyslog
echo "*.* @@siem-server:514" >> /etc/rsyslog.conf

# Or use journald forwarding
journalctl -u ssh-vpn -o json | jq -c '.' | nc siem-server 514
```

---

## Compliance

### GDPR
- All data encrypted in transit (AES-256-GCM)
- No data stored on VPN server (stateless)
- Access logs retained per policy

### HIPAA
- AES-256 encryption meets requirements
- Audit logging of all connections
- Key management via SSH key rotation

### SOC 2
- Encryption in transit: AES-256-GCM
- Access control: Key-based authentication
- Monitoring: Prometheus metrics + logs

---

## Key Rotation

Rotate keys periodically:

```bash
# Generate new key
ssh-keygen -t ed25519 -f ~/.ssh/vpn_key_new -N ""

# Add new key to server
echo "ssh-ed25519 AAAA... new@host" >> authorized_keys

# Update client config
# client:
#   private_key_path: "~/.ssh/vpn_key_new"

# Remove old key from server
sed -i '/old@host/d' authorized_keys

# Remove old key from client
rm ~/.ssh/vpn_key ~/.ssh/vpn_key.pub
```

---

## Reporting Vulnerabilities

If you discover a security vulnerability:

1. **Don't** open a public GitHub issue
2. Email: security@ssh-vpn.example.com (or open a private issue)
3. Include: description, steps to reproduce, impact assessment
4. We'll respond within 48 hours

See also: [GitHub Security Policy](https://github.com/nnip7777/ssh-vpn/security)
