import SwiftUI
import UniformTypeIdentifiers

struct ContentView: View {
    @StateObject private var vpn = VPNManager()
    @State private var importing = false

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 16) {
                Text(vpn.statusText)
                    .font(.body)
                Text(vpn.pingText)
                    .font(.body)
                    .foregroundStyle(.secondary)
                Text(vpn.versionLabel)
                    .font(.caption)
                    .foregroundStyle(.secondary)

                HStack {
                    Text("MTU")
                    TextField("1400", text: $vpn.mtuText)
                        .keyboardType(.numberPad)
                        .textFieldStyle(.roundedBorder)
                        .frame(maxWidth: 100)
                        .disabled(vpn.connected)
                }

                if let err = vpn.lastError, !err.isEmpty {
                    Text(err)
                        .font(.footnote)
                        .foregroundStyle(.red)
                }

                Button("Import Profile") { importing = true }
                    .buttonStyle(.bordered)
                    .disabled(vpn.connected)

                Button(vpn.connectTitle) {
                    vpn.toggle()
                }
                .buttonStyle(.borderedProminent)
                .disabled(vpn.busy)

                Spacer()
            }
            .padding()
            .navigationTitle("MASQUE")
            .fileImporter(isPresented: $importing, allowedContentTypes: [.item, .text, .data]) { result in
                if case .success(let url) = result {
                    vpn.importFile(url: url)
                }
            }
        }
    }
}
