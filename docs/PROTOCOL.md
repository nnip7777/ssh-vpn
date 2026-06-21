# SSH VPN Protocol Specification

## Overview

The SSH VPN protocol is designed to provide secure, multi-channel VPN connectivity with load balancing and fault tolerance. It operates over standard SSH connections with custom channel types.

## Protocol Stack

```
┌─────────────────────────────────────────┐
│           Application Layer             │
│    (VPN traffic: IP packets)            │
├─────────────────────────────────────────┤
│           Channel Layer                 │
│    (Multiplexing, Load Balancing)       │
├─────────────────────────────────────────┤
│           SSH Transport Layer           │
│    (Encryption, Authentication)         │
├─────────────────────────────────────────┤
│           Network Layer                 │
│    (TCP/IP)                             │
└─────────────────────────────────────────┘
```

## Channel Types

### Read Channel (Type: 0x01)
- Used for incoming data (downloads, streaming)
- Typically 80% of bandwidth allocation
- Multiple read channels for redundancy

### Write Channel (Type: 0x02)
- Used for outgoing data (requests, ACKs)
- Typically 20% of bandwidth allocation
- Fewer write channels needed

### Control Channel (Type: 0x03)
- Used for management and heartbeat
- Single channel per client
- Carries statistics and health information

## Packet Format

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|         Channel ID            |   Channel   |    Message    |
|                               |    Type     |      Type     |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         Sequence Number                       |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         Data Length                           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
|                         Data Payload                          |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

### Header Fields

| Field           | Size    | Description                           |
|-----------------|---------|---------------------------------------|
| Channel ID      | 2 bytes | Unique channel identifier             |
| Channel Type    | 1 byte  | 0x01=Read, 0x02=Write, 0x03=Control  |
| Message Type    | 1 byte  | 0x01=Data, 0x02=Heartbeat, etc.     |
| Sequence Number | 4 bytes | Packet sequence number                |
| Data Length     | 4 bytes | Length of data payload                |

## Message Types

| Type   | Value | Description                    |
|--------|-------|--------------------------------|
| Data   | 0x01  | Regular data packet            |
| Heartbeat | 0x02 | Keep-alive message          |
| Stats  | 0x03  | Statistics exchange            |
| Handshake | 0x04 | Initial connection setup   |

## Handshake Process

1. Client connects via SSH
2. Client opens control channel
3. Client sends Handshake message:
   - Protocol version
   - Client ID
   - Requested channel configuration
   - Read/Write ratios
4. Server responds with accepted configuration
5. Server opens requested channels
6. Data transfer begins

## Load Balancing Algorithm

### Weighted Round Robin
- Each channel has a weight based on:
  - Latency (lower = higher weight)
  - Bandwidth (higher = higher weight)
  - Packet loss (lower = higher weight)
- Channels are selected proportionally to weight

### Health Monitoring
- Heartbeat every 5 seconds
- Channel marked unhealthy if:
  - No response for 30 seconds
  - Packet loss > 10%
  - Latency > 100ms
- Unhealthy channels are removed from pool
- New channels created if below minimum

## Fault Tolerance

### Automatic Failover
1. Channel failure detected
2. Traffic immediately rerouted to healthy channels
3. Connection re-established in background
4. Statistics reset for new channel

### Dynamic Scaling
- Minimum channels maintained at all times
- Additional channels created under high load
- Channels removed when load decreases
- Scaling decisions based on:
  - Bandwidth utilization
  - Latency measurements
  - Error rates

## Security

### Encryption
- All traffic encrypted via SSH
- Default: AES-256-GCM
- Key exchange: Curve25519

### Authentication
- Public key authentication (recommended)
- Password authentication (fallback)
- Certificate-based authentication (enterprise)

### Compression
- Optional LZ4 compression
- Reduces bandwidth usage
- Configurable per channel

## Flow Control

### Backpressure
- Channels implement flow control
- Slow channels receive fewer packets
- Fast channels handle more load
- Prevents buffer overflow

### Congestion Control
- Adaptive window sizing
- Based on RTT measurements
- Similar to TCP congestion control
