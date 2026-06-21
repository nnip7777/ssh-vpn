# SSH VPN Architecture

## System Overview

SSH VPN provides secure, multi-channel VPN connectivity with built-in load balancing and fault tolerance. The system consists of a Linux server and cross-platform clients.

## Components

### Server (Linux)

```
┌─────────────────────────────────────────────────────────┐
│                    SSH VPN Server                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │              SSH Connection Handler              │   │
│  │  - Accepts incoming connections                  │   │
│  │  - Authenticates clients                         │   │
│  │  - Manages SSH sessions                          │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         ▼                               │
│  ┌─────────────────────────────────────────────────┐   │
│  │              Channel Manager                     │   │
│  │  - Creates/manages Read channels                 │   │
│  │  - Creates/manages Write channels                │   │
│  │  - Monitors channel health                       │   │
│  │  - Handles failover                              │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         ▼                               │
│  ┌─────────────────────────────────────────────────┐   │
│  │              Load Balancer                       │   │
│  │  - Distributes traffic across channels           │   │
│  │  - Implements weighted round-robin               │   │
│  │  - Adjusts weights based on performance          │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         ▼                               │
│  ┌─────────────────────────────────────────────────┐   │
│  │              TUN/TAP Interface                   │   │
│  │  - Virtual network adapter                       │   │
│  │  - Captures IP packets                           │   │
│  │  - Routes traffic to channels                    │   │
│  └─────────────────────────────────────────────────┘   │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Client (Cross-Platform)

```
┌─────────────────────────────────────────────────────────┐
│                    SSH VPN Client                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │              Application Layer                   │   │
│  │  - User applications                             │   │
│  │  - System network stack                          │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         ▼                               │
│  ┌─────────────────────────────────────────────────┐   │
│  │              TUN/TAP Interface                   │   │
│  │  - Virtual network adapter                       │   │
│  │  - Captures outgoing packets                     │   │
│  │  - Delivers incoming packets                     │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         ▼                               │
│  ┌─────────────────────────────────────────────────┐   │
│  │              Load Balancer                       │   │
│  │  - Selects best channel for each packet          │   │
│  │  - Handles failover on channel failure           │   │
│  │  - Balances load across channels                 │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         ▼                               │
│  ┌─────────────────────────────────────────────────┐   │
│  │              Channel Manager                     │   │
│  │  - Maintains pool of channels                    │   │
│  │  - Creates new channels as needed                │   │
│  │  - Removes failed channels                       │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         ▼                               │
│  ┌─────────────────────────────────────────────────┐   │
│  │              SSH Connection Handler              │   │
│  │  - Establishes connection to server              │   │
│  │  - Handles reconnection                          │   │
│  │  - Manages encryption/decryption                 │   │
│  └─────────────────────────────────────────────────┘   │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Data Flow

### Outgoing Traffic (Client → Server)

1. Application sends IP packet
2. TUN interface captures packet
3. Load balancer selects best Write channel
4. Packet encrypted via SSH
5. Sent over selected channel
6. Server receives on TUN interface
7. Server routes to destination

### Incoming Traffic (Server → Client)

1. Server receives IP packet
2. Packet encrypted via SSH
3. Load balancer selects best Read channel
4. Sent over selected channel
5. Client receives on TUN interface
6. Client routes to application
7. Application receives packet

## Channel Management

### Creation
- Minimum channels created at startup
- Additional channels created when:
  - Load exceeds threshold
  - Minimum not met after failure
  - Client requests more channels

### Monitoring
- Heartbeat every 5 seconds
- Latency measurement
- Packet loss tracking
- Bandwidth utilization

### Removal
- Channel marked unhealthy after:
  - 3 consecutive missed heartbeats
  - Packet loss > 10%
  - Latency > 100ms
- Traffic rerouted immediately
- Channel closed and removed

## Load Balancing

### Algorithm: Weighted Round Robin

Weight calculation:
```
weight = (1 / latency) * bandwidth * (1 - packet_loss)
```

Selection:
1. Calculate weight for each channel
2. Select channel with highest weight
3. Adjust weight based on usage
4. Prevent starvation with minimum allocation

### Read/Write Split

- Read channels: 80% of bandwidth
  - Used for downloads, streaming
  - More channels for redundancy
- Write channels: 20% of bandwidth
  - Used for requests, ACKs
  - Fewer channels sufficient

## Fault Tolerance

### Detection
- Heartbeat timeout (15 seconds)
- Error rate threshold (10%)
- Latency threshold (100ms)

### Recovery
1. Detect failure
2. Mark channel as failed
3. Remove from channel pool
4. Reroute traffic to healthy channels
5. Attempt reconnection in background
6. Add channel back when healthy

### Scaling
- Scale up: When load > 80% capacity
- Scale down: When load < 20% capacity
- Minimum channels always maintained
- Maximum channels limited by config

## Security Model

### Encryption
- All traffic encrypted via SSH
- AES-256-GCM for data
- Curve25519 for key exchange

### Authentication
- Public key (primary)
- Password (fallback)
- Certificate (enterprise)

### Access Control
- Per-user channel limits
- IP-based restrictions
- Time-based access

## Platform Support

### Server
- Linux only (requires TUN/TAP kernel support)
- amd64, arm64 architectures

### Clients
- macOS: Native Go binary
- Windows: Native Go binary
- Linux: Native Go binary
- iOS: Go mobile + Swift wrapper
- Android: Go mobile + Kotlin wrapper

## Performance Considerations

### Optimization
- LZ4 compression for bandwidth savings
- Packet batching for efficiency
- Channel reuse for connection pooling
- Adaptive MTU for path optimization

### Monitoring
- Real-time statistics via control channel
- Prometheus metrics export
- Grafana dashboard support
- Log aggregation
