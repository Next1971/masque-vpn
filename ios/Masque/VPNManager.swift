import Combine
import Foundation
import NetworkExtension

final class VPNManager: ObservableObject {
    static let providerBundleID = "com.next1971.masque.packet-tunnel"

    @Published var statusText = "Status: profile not configured"
    @Published var pingText = "Ping: —"
    @Published var connected = false
    @Published var mtuText = "1400"
    @Published var lastError: String?

    private var observer: NSObjectProtocol?
    private var pingTimer: Timer?

    init() {
        refreshProfileStatus()
        observer = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            self?.applyConnectionStatus()
        }
        pingTimer = Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { [weak self] _ in
            self?.refreshPing()
        }
        loadExistingManager { [weak self] in
            self?.applyConnectionStatus()
        }
    }

    deinit {
        if let observer { NotificationCenter.default.removeObserver(observer) }
        pingTimer?.invalidate()
    }

    var versionLabel: String {
        let name = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "?"
        let code = Bundle.main.infoDictionary?["CFBundleVersion"] as? String ?? "?"
        return "v\(name) (\(code))"
    }

    func refreshProfileStatus() {
        if ProfileStore.isConfigured() {
            if !connected {
                statusText = "Status: profile ready"
            }
        } else {
            statusText = "Status: profile not configured"
        }
        let stored = AppGroup.defaults.object(forKey: AppGroup.defaultsMTU) as? Int ?? 1400
        mtuText = String(stored)
    }

    func importFile(url: URL) {
        lastError = nil
        let accessed = url.startAccessingSecurityScopedResource()
        defer {
            if accessed { url.stopAccessingSecurityScopedResource() }
        }
        do {
            let text = try String(contentsOf: url, encoding: .utf8)
            try ProfileStore.importText(text)
            refreshProfileStatus()
        } catch {
            lastError = error.localizedDescription
        }
    }

    func toggle() {
        lastError = nil
        if connected {
            loadExistingManager { mgr in
                mgr?.connection.stopVPNTunnel()
            }
            return
        }
        guard ProfileStore.isConfigured(), let profile = ProfileStore.load() else {
            lastError = "Import a profile first"
            return
        }
        let mtu = clampMTU(Int(mtuText) ?? 1400)
        mtuText = String(mtu)
        ProfileStore.saveMTU(mtu)

        ensureManager(server: profile.server) { [weak self] result in
            switch result {
            case .failure(let err):
                DispatchQueue.main.async { self?.lastError = err.localizedDescription }
            case .success(let mgr):
                do {
                    try mgr.connection.startVPNTunnel()
                } catch {
                    DispatchQueue.main.async { self?.lastError = error.localizedDescription }
                }
            }
        }
    }

    private func clampMTU(_ v: Int) -> Int {
        min(1500, max(1280, v))
    }

    private func applyConnectionStatus() {
        loadExistingManager { [weak self] mgr in
            guard let self else { return }
            let status = mgr?.connection.status ?? .invalid
            DispatchQueue.main.async {
                switch status {
                case .connected:
                    self.connected = true
                    self.statusText = "Status: VPN active"
                case .connecting, .reasserting:
                    self.connected = false
                    self.statusText = "Status: connecting"
                case .disconnecting:
                    self.connected = false
                    self.statusText = "Status: disconnecting"
                default:
                    self.connected = false
                    self.pingText = "Ping: —"
                    self.refreshProfileStatus()
                }
            }
        }
    }

    private func refreshPing() {
        guard connected else { return }
        let ms = AppGroup.defaults.integer(forKey: AppGroup.defaultsPing)
        let msg = AppGroup.defaults.string(forKey: AppGroup.defaultsStatus) ?? ""
        if ms > 0 {
            pingText = "Ping: \(ms) ms"
            if !msg.isEmpty { statusText = "Status: \(msg)" }
        }
    }

    private func loadExistingManager(completion: @escaping (NETunnelProviderManager?) -> Void) {
        NETunnelProviderManager.loadAllFromPreferences { list, _ in
            let match = (list ?? []).first {
                ($0.protocolConfiguration as? NETunnelProviderProtocol)?.providerBundleIdentifier == VPNManager.providerBundleID
            }
            completion(match)
        }
    }

    private func loadExistingManager(then: @escaping () -> Void) {
        loadExistingManager { _ in then() }
    }

    private func ensureManager(server: String, completion: @escaping (Result<NETunnelProviderManager, Error>) -> Void) {
        loadExistingManager { existing in
            let mgr = existing ?? NETunnelProviderManager()
            let proto = NETunnelProviderProtocol()
            proto.providerBundleIdentifier = VPNManager.providerBundleID
            proto.serverAddress = server
            mgr.protocolConfiguration = proto
            mgr.localizedDescription = "MASQUE"
            mgr.isEnabled = true
            mgr.saveToPreferences { err in
                if let err {
                    completion(.failure(err))
                    return
                }
                mgr.loadFromPreferences { loadErr in
                    if let loadErr {
                        completion(.failure(loadErr))
                    } else {
                        completion(.success(mgr))
                    }
                }
            }
        }
    }
}
