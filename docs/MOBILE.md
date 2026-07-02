# SSH VPN - Mobile Integration

## iOS Integration

### Prerequisites
- macOS with Xcode 14+
- Go 1.22+
- gomobile

### Build Framework
```bash
./build_ios.sh
```

### Xcode Integration
1. Open your Xcode project
2. Drag `build/SSHVPN.xcframework` into the project
3. In Build Settings, add to Framework Search Paths: `$(PROJECT_DIR)/build`
4. Import and use:

```swift
import SSHVPN

class VPNManager: ObservableObject {
    @Published var status = "disconnected"
    private var client: SSHTunnel?

    func connect(server: String, port: Int32, username: String, password: String) {
        let config = SSHVPNDefaultVPNConfig()
        config.serverAddr = server
        config.serverPort = port
        config.username = username
        config.password = password
        config.tunName = "tun0"
        config.tunAddr = "10.0.0.2"
        config.compression = "lz4"

        client = try SSHVPNNewVPNClient(config)
        client?.setOnStatus { [weak self] newStatus in
            DispatchQueue.main.async {
                self?.status = newStatus
            }
        }

        try client?.connect()
        try client?.startTunnel()
    }

    func disconnect() {
        client?.disconnect()
    }
}
```

### Network Extension (iOS 14+)
For production iOS apps, use NetworkExtension framework:

```swift
import NetworkExtension
import SSHVPN

class PacketTunnelProvider: NEPacketTunnelProvider {
    override func startTunnel(options: [String: NSObject]?,
                              completionHandler: @escaping (Error?) -> Void) {
        let config = SSHVPNDefaultVPNConfig()
        // Configure...
        
        let client = try SSHVPNNewVPNClient(config)
        try client.connect()
        try client.startTunnel()
        
        completionHandler(nil)
    }
    
    override func stopTunnel(with reason: NEProviderStopReason,
                             completionHandler: @escaping () -> Void) {
        client?.disconnect()
        completionHandler()
    }
}
```

---

## Android Integration

### Prerequisites
- Android Studio
- Android SDK (API 24+)
- Go 1.22+
- gomobile
- ANDROID_HOME set

### Build AAR
```bash
./build_android.sh
```

### Android Studio Integration
1. Copy `build/ssh-vpn.aar` to `app/libs/`
2. In `build.gradle`:
```gradle
dependencies {
    implementation files('libs/ssh-vpn.aar')
}
```
3. Use in Kotlin:

```kotlin
import lib

class VPNManager {
    private var client: lib.VPNClient?

    fun connect(serverAddr: String, serverPort: Int, username: String, password: String,
                callback: (Boolean, String) -> Unit) {
        val config = lib.DefaultVPNConfig()
        config.serverAddr = serverAddr
        config.serverPort = serverPort.toLong()
        config.username = username
        config.password = password
        config.tunName = "tun0"
        config.tunAddr = "10.0.0.2"
        config.compression = "lz4"

        client = lib.NewVPNClient(config)
        client?.setOnStatus { status ->
            // Handle status update
        }

        val result = client?.connect()
        if (result != null) {
            client?.startTunnel()
            callback(true, "Connected")
        } else {
            callback(false, "Connection failed")
        }
    }

    fun disconnect() {
        client?.disconnect()
    }
}
```

### Android VPN Service
```kotlin
import android.net.VpnService
import lib

class MyVPNService : VpnService() {
    private var client: lib.VPNClient?

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val config = lib.DefaultVPNConfig()
        config.serverAddr = "your-server.com"
        config.serverPort = 2222
        config.username = "user"
        config.password = "pass"

        client = lib.NewVPNClient(config)
        client?.connect()
        client?.startTunnel()

        return START_STICKY
    }

    override fun onDestroy() {
        client?.disconnect()
        super.onDestroy()
    }
}
```

---

## API Reference

### VPNConfig
| Field | Type | Default | Description |
|-------|------|---------|-------------|
| ServerAddr | String | "localhost" | Server address |
| ServerPort | Int | 2222 | Server port |
| Username | String | "" | SSH username |
| Password | String | "" | SSH password |
| PrivateKeyPath | String | "" | SSH private key path |
| TUNName | String | "tun0" | TUN interface name |
| TUNAddr | String | "10.0.0.2" | TUN IP address |
| TUNNetmask | String | "255.255.255.0" | TUN netmask |
| MTU | Int | 1280 | Maximum transmission unit |
| MinRead | Int | 2 | Minimum read channels |
| MaxRead | Int | 8 | Maximum read channels |
| MinWrite | Int | 1 | Minimum write channels |
| MaxWrite | Int | 4 | Maximum write channels |
| ReadRatio | Float | 0.8 | Read channel ratio (80%) |
| WriteRatio | Float | 0.2 | Write channel ratio (20%) |
| Compression | String | "lz4" | Compression algorithm |

### VPNClient Methods
| Method | Description |
|--------|-------------|
| Connect() | Connect to VPN server |
| StartTunnel() | Start VPN tunnel |
| Disconnect() | Disconnect from VPN |
| GetStatus() | Get connection status |
| IsConnected() | Check if connected |
| GetChannelStats() | Get channel statistics |
| SetOnStatus(callback) | Set status callback |
| SetOnError(callback) | Set error callback |
| SetOnStats(callback) | Set statistics callback |

### Status Values
- `disconnected` - Not connected
- `connecting` - Establishing connection
- `connected` - Connected to server
- `tunnel_active` - VPN tunnel active
- `reconnecting` - Attempting to reconnect
- `error` - Connection error
