import SwiftUI
import UniformTypeIdentifiers

struct ContentView: View {
    @StateObject private var vpn = VPNManager()
    @State private var importing = false
    @State private var scanning = false
    @State private var showSettings = false

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 16) {
                Text(vpn.statusText)
                    .font(.body)
                Text(vpn.pingText)
                    .font(.body)
                    .foregroundStyle(.secondary)
                Text(vpn.trafficText)
                    .font(.body)
                    .foregroundStyle(.secondary)
                Text(vpn.versionLabel)
                    .font(.caption)
                    .foregroundStyle(.secondary)

                if let err = vpn.lastError, !err.isEmpty {
                    Text(err)
                        .font(.footnote)
                        .foregroundStyle(.red)
                }

                Button("Import Profile") { importing = true }
                    .buttonStyle(.bordered)
                    .disabled(vpn.connected)

                Button("Scan QR") { scanning = true }
                    .buttonStyle(.bordered)
                    .disabled(vpn.connected)

                Button(vpn.connectTitle) {
                    vpn.toggle()
                }
                .buttonStyle(.borderedProminent)

                Button("Settings") { showSettings = true }
                    .buttonStyle(.bordered)

                Spacer()
            }
            .padding()
            .navigationTitle("MASQUE")
            .fileImporter(isPresented: $importing, allowedContentTypes: [.item, .text, .data]) { result in
                if case .success(let url) = result {
                    vpn.importFile(url: url)
                }
            }
            .sheet(isPresented: $showSettings) {
                SettingsView(vpn: vpn)
            }
            .sheet(isPresented: $scanning) {
                NavigationStack {
                    QRScannerView { text in
                        scanning = false
                        vpn.importText(text)
                    } onFailure: { message in
                        scanning = false
                        vpn.lastError = message
                    }
                    .ignoresSafeArea()
                    .navigationTitle("Scan profile")
                    .navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItem(placement: .cancellationAction) {
                            Button("Cancel") { scanning = false }
                        }
                    }
                }
            }
        }
    }
}

private struct SettingsView: View {
    @ObservedObject var vpn: VPNManager
    @Environment(\.dismiss) private var dismiss
    @State private var transport: String
    @State private var killSwitch: Bool
    @State private var mtu: String

    init(vpn: VPNManager) {
        self.vpn = vpn
        _transport = State(initialValue: vpn.transport)
        _killSwitch = State(initialValue: vpn.killSwitch)
        _mtu = State(initialValue: vpn.mtuText)
    }

    var body: some View {
        NavigationStack {
            Form {
                Picker("Transport", selection: $transport) {
                    Text("Automatic (UDP, then TCP)").tag("auto")
                    Text("UDP only").tag("udp")
                    Text("TCP only").tag("tcp")
                }
                Toggle("Kill switch (block internet if the tunnel drops)", isOn: $killSwitch)
                TextField("MTU", text: $mtu)
                    .keyboardType(.numberPad)
                Text("Applies on the next connect. MTU 1280–1500, default 1340. Kill switch blocks internet if the tunnel drops, until MASQUE reconnects or you disconnect. If you turn the VPN off in iOS Settings, open MASQUE once to restore internet.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        vpn.saveSettings(transport: transport, killSwitch: killSwitch, mtuText: mtu)
                        dismiss()
                    }
                }
            }
        }
    }
}
