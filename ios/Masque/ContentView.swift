import SwiftUI
import UniformTypeIdentifiers

struct ContentView: View {
    @StateObject private var vpn = VPNManager()
    @State private var importing = false
    @State private var scanning = false
    @State private var showSettings = false
    @State private var switchCandidate: ProfileStore.ProfileInfo?
    @State private var deleteCandidate: ProfileStore.ProfileInfo?

    private var activeInfo: ProfileStore.ProfileInfo? {
        vpn.profiles.first(where: { $0.isActive })
    }

    private var profileSelection: Binding<String> {
        Binding(
            get: { vpn.profiles.first(where: { $0.isActive })?.id ?? "" },
            set: { newID in
                guard let info = vpn.profiles.first(where: { $0.id == newID }), !info.isActive else { return }
                switchCandidate = info
            }
        )
    }

    /// Duplicate profile names get the server address appended.
    private func label(_ info: ProfileStore.ProfileInfo) -> String {
        let dup = vpn.profiles.count(where: { $0.name == info.name }) > 1
        return dup && !info.server.isEmpty ? "\(info.name) — \(info.server)" : info.name
    }

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 16) {
                Text(vpn.statusText)
                    .font(.body)
                    .lineLimit(1)
                    .frame(maxWidth: .infinity, alignment: .leading)
                Text(vpn.pingText)
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                Text(vpn.trafficText)
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                Text(vpn.versionLabel)
                    .font(.caption)
                    .foregroundStyle(.secondary)

                if let err = vpn.lastError, !err.isEmpty {
                    Text(err)
                        .font(.footnote)
                        .foregroundStyle(.red)
                }

                HStack {
                    Picker("Profile", selection: profileSelection) {
                        if vpn.profiles.isEmpty {
                            Text("No profile").tag("")
                        }
                        ForEach(vpn.profiles, id: \.id) { info in
                            Text(label(info)).tag(info.id)
                        }
                    }
                    .pickerStyle(.menu)

                    Button("Delete") {
                        deleteCandidate = activeInfo
                    }
                    .buttonStyle(.bordered)
                    .disabled(activeInfo == nil || vpn.connected || vpn.busy)
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
            .onAppear { vpn.refreshProfileStatus() }
            .confirmationDialog(
                "Switch profile?", isPresented: Binding(get: { switchCandidate != nil }, set: { if !$0 { switchCandidate = nil } }),
                titleVisibility: .visible, presenting: switchCandidate
            ) { info in
                Button("Switch") { vpn.switchProfile(to: info) }
                Button("Cancel", role: .cancel) {}
            } message: { info in
                Text("Switching to \"\(info.name)\" disconnects the current VPN session.")
            }
            .alert(
                "Delete profile?", isPresented: Binding(get: { deleteCandidate != nil }, set: { if !$0 { deleteCandidate = nil } }),
                presenting: deleteCandidate
            ) { info in
                Button("Delete", role: .destructive) { vpn.deleteProfile(info) }
                Button("Cancel", role: .cancel) {}
            } message: { info in
                Text("Delete \"\(info.name)\"? Its certificates are removed from this device. The profile file you imported stays where it is.")
            }
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
