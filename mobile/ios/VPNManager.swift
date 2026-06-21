import SSHVPN

class VPNManager: ObservableObject {
    @Published var status = "disconnected"
    @Published var isConnected = false

    private var client: SSHVPNVPNClient?

    func connect(server: String, port: Int32, username: String, password: String) {
        let config = SSHVPNDefaultVPNConfig()
        config.serverAddr = server
        config.serverPort = port
        config.username = username
        config.password = password
        config.tunName = "tun0"
        config.tunAddr = "10.0.0.2"
        config.tunNetmask = "255.255.255.0"
        config.mtu = 1400
        config.compression = "lz4"

        do {
            client = try SSHVPNNewVPNClient(config)
            client?.setOnStatus({ [weak self] newStatus in
                DispatchQueue.main.async {
                    self?.status = newStatus
                    self?.isConnected = newStatus == "connected"
                }
            })
            client?.setOnError({ error in
                print("VPN Error: \(error)")
            })

            try client?.connect()
            try client?.startTunnel()
        } catch {
            print("Failed to connect: \(error)")
        }
    }

    func disconnect() {
        client?.disconnect()
        status = "disconnected"
        isConnected = false
    }

    func getChannelStats() -> [String: Any]? {
        return client?.getChannelStats() as? [String: Any]
    }
}
