package com.sshvpn.lib

import lib.VPNClient
import lib.VPNConfig
import lib.DefaultVPNConfig
import lib.NewVPNClient

class VPNManager {
    private var client: VPNClient? = null
    private var statusCallback: ((String) -> Unit)? = null
    private var errorCallback: ((String) -> Unit)? = null

    fun setStatusCallback(callback: (String) -> Unit) {
        statusCallback = callback
    }

    fun setErrorCallback(callback: (String) -> Unit) {
        errorCallback = callback
    }

    fun connect(
        serverAddr: String,
        serverPort: Int,
        username: String,
        password: String,
        callback: (Boolean, String) -> Unit
    ) {
        try {
            val config: VPNConfig = DefaultVPNConfig()
            config.setServerAddr(serverAddr)
            config.setServerPort(serverPort.toLong())
            config.setUsername(username)
            config.setPassword(password)
            config.setTunName("tun0")
            config.setTunAddr("10.0.0.2")
            config.setTunNetmask("255.255.255.0")
            config.setMtu(1400)
            config.setCompression("lz4")

            client = NewVPNClient(config)

            client?.setOnStatus { status ->
                statusCallback?.invoke(status)
            }

            client?.setOnError { error ->
                errorCallback?.invoke(error)
            }

            val connectResult = client?.connect()
            if (connectResult != null) {
                client?.startTunnel()
                callback(true, "Connected")
            } else {
                callback(false, "Connection failed")
            }
        } catch (e: Exception) {
            callback(false, e.message ?: "Unknown error")
        }
    }

    fun disconnect() {
        client?.disconnect()
        client = null
        statusCallback?.invoke("disconnected")
    }

    fun getStatus(): String {
        return client?.getStatus() ?: "disconnected"
    }

    fun isConnected(): Boolean {
        return client?.isConnected() ?: false
    }

    fun getChannelStats(): Map<String, Any>? {
        return client?.getChannelStats()
    }
}
