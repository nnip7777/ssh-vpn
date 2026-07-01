import SwiftUI

struct ContentView: View {
    @State private var server = ""
    @State private var port = "22"
    @State private var username = ""
    @State private var password = ""
    @State private var status = "disconnected"
    @State private var connected = false

    var body: some View {
        VStack(spacing: 16) {
            Text("SSH VPN")
                .font(.largeTitle)
                .fontWeight(.bold)

            TextField("Server (IP или домен)", text: $server)
                .textFieldStyle(.roundedBorder)

            TextField("Port", text: $port)
                .textFieldStyle(.roundedBorder)

            TextField("Username", text: $username)
                .textFieldStyle(.roundedBorder)

            SecureField("Password", text: $password)
                .textFieldStyle(.roundedBorder)

            Button(action: {
                if connected {
                    disconnect()
                } else {
                    connect()
                }
            }) {
                Text(connected ? "Disconnect" : "Connect")
                    .frame(maxWidth: .infinity)
                    .padding()
                    .background(connected ? Color.red : Color.blue)
                    .foregroundColor(.white)
                    .cornerRadius(10)
            }

            Text("Status: \(status)")
                .foregroundColor(.gray)

            Spacer()
        }
        .padding()
    }

    func connect() {
        status = "connecting"
        connected = true
        // TODO: integrate SSHVPN framework
    }

    func disconnect() {
        status = "disconnected"
        connected = false
    }
}
