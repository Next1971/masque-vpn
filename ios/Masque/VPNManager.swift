import Combine
import Foundation
import NetworkExtension

final class VPNManager: ObservableObject {
    static let providerBundleID = "com.next1971.masque.packet-tunnel"

    @Published var statusText = "Status: profile not configured"
    @Published var pingText = "Ping: —"
    @Published var connected = false
    @Published var busy = false
    @Published var mtuText = "1400"
    @Published var lastError: String?

    private var observer: NSObjectProtocol?
    private var pingTimer: Timer?
    private var busyWatchdog: Timer?
    private let prefs = DispatchQueue(label: "com.next1971.masque.vpn-prefs")
    /// Only touch on `prefs` queue.
    private var cachedManager: NETunnelProviderManager?

    init() {
        refreshProfileStatus()
        observer = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange,
            object: nil,
            queue: .main
        ) { [weak self] note in
            self?.applyConnectionStatus(from: note.object as? NEVPNConnection)
        }
        pingTimer = Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { [weak self] _ in
            self?.refreshPing()
        }
        prefs.async { [weak self] in
            self?.reloadManager { _ in
                DispatchQueue.main.async { self?.applyConnectionStatus(from: nil) }
            }
        }
    }

    deinit {
        if let observer { NotificationCenter.default.removeObserver(observer) }
        pingTimer?.invalidate()
        busyWatchdog?.invalidate()
    }

    var versionLabel: String {
        let name = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "?"
        let code = Bundle.main.infoDictionary?["CFBundleVersion"] as? String ?? "?"
        return "v\(name) (\(code))"
    }

    var connectTitle: String {
        if busy { return connected ? "Disconnecting…" : "Connecting… (tap to stop)" }
        return connected ? "Disconnect" : "Connect"
    }

    func refreshProfileStatus() {
        if ProfileStore.isConfigured() {
            if !connected && !busy {
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
        // A stuck busy flag used to make the button do nothing at all, so a tap
        // while busy stops whatever is half-started instead of being ignored.
        if busy {
            busy = false
            statusText = "Status: cancelling"
            prefs.async { [weak self] in
                self?.reloadManager { mgr in
                    mgr?.connection.stopVPNTunnel()
                    DispatchQueue.main.async { self?.refreshProfileStatus() }
                }
            }
            return
        }

        if connected {
            busy = true
            statusText = "Status: disconnecting"
            prefs.async { [weak self] in
                self?.reloadManager { mgr in
                    mgr?.connection.stopVPNTunnel()
                    DispatchQueue.main.async {
                        self?.busy = false
                    }
                }
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

        busy = true
        statusText = "Status: starting…"

        prefs.async { [weak self] in
            self?.startTunnel(server: profile.server, attempt: 0)
        }
    }

    private func clampMTU(_ v: Int) -> Int {
        min(1500, max(1280, v))
    }

    /// The extension can die without a final status change, so busy is never
    /// left on forever; otherwise Connect stays greyed out until app restart.
    private func setBusy(_ on: Bool) {
        busy = on
        busyWatchdog?.invalidate()
        busyWatchdog = nil
        guard on else { return }
        busyWatchdog = Timer.scheduledTimer(withTimeInterval: 25, repeats: false) { [weak self] _ in
            guard let self, self.busy, !self.connected else { return }
            self.busy = false
            let tunnelErr = AppGroup.defaults.string(forKey: AppGroup.defaultsLastError) ?? ""
            self.lastError = tunnelErr.isEmpty ? "Connect timed out" : tunnelErr
            self.refreshProfileStatus()
        }
    }

    private func startTunnel(server: String, attempt: Int) {
        prepareManager(server: server) { [weak self] result in
            guard let self else { return }
            switch result {
            case .failure(let err):
                DispatchQueue.main.async {
                    self.busy = false
                    self.lastError = err.localizedDescription
                    self.refreshProfileStatus()
                }
            case .success(let mgr):
                do {
                    try mgr.connection.startVPNTunnel()
                    DispatchQueue.main.async {
                        self.applyConnectionStatus(from: mgr.connection)
                    }
                } catch {
                    let ns = error as NSError
                    if ns.domain == NEVPNErrorDomain,
                       ns.code == NEVPNError.configurationStale.rawValue,
                       attempt < 2 {
                        self.cachedManager = nil
                        self.startTunnel(server: server, attempt: attempt + 1)
                        return
                    }
                    if ns.domain == NEVPNErrorDomain,
                       ns.code == NEVPNError.configurationStale.rawValue ||
                       ns.code == NEVPNError.configurationInvalid.rawValue {
                        self.recreateManager(server: server, attempt: attempt)
                        return
                    }
                    DispatchQueue.main.async {
                        self.busy = false
                        self.lastError = error.localizedDescription
                        self.refreshProfileStatus()
                    }
                }
            }
        }
    }

    private func recreateManager(server: String, attempt: Int) {
        reloadManager { [weak self] mgr in
            guard let self else { return }
            let finishFail: (Error) -> Void = { err in
                DispatchQueue.main.async {
                    self.busy = false
                    self.lastError = err.localizedDescription
                    self.refreshProfileStatus()
                }
            }
            guard let mgr else {
                if attempt < 2 {
                    self.startTunnel(server: server, attempt: attempt + 1)
                } else {
                    finishFail(NSError(
                        domain: "masque",
                        code: 4,
                        userInfo: [NSLocalizedDescriptionKey: "VPN configuration missing"]
                    ))
                }
                return
            }
            mgr.removeFromPreferences { remErr in
                self.prefs.async {
                    self.cachedManager = nil
                    if let remErr {
                        finishFail(remErr)
                        return
                    }
                    if attempt < 2 {
                        self.startTunnel(server: server, attempt: attempt + 1)
                    } else {
                        finishFail(NSError(
                            domain: "masque",
                            code: 4,
                            userInfo: [NSLocalizedDescriptionKey: "VPN configuration is stale"]
                        ))
                    }
                }
            }
        }
    }

    /// load → mutate → save → loadAll (fresh) before start.
    private func prepareManager(server: String, completion: @escaping (Result<NETunnelProviderManager, Error>) -> Void) {
        reloadManager { [weak self] existing in
            guard let self else { return }
            let mgr = existing ?? NETunnelProviderManager()
            // Apple: must load before the first save after launch / on a new manager.
            mgr.loadFromPreferences { loadErr in
                self.prefs.async {
                    if let loadErr,
                       (loadErr as NSError).domain == NEVPNErrorDomain,
                       (loadErr as NSError).code != NEVPNError.configurationInvalid.rawValue {
                        // Invalid is expected for a brand-new never-saved manager.
                        completion(.failure(loadErr))
                        return
                    }
                    let proto = NETunnelProviderProtocol()
                    proto.providerBundleIdentifier = VPNManager.providerBundleID
                    proto.serverAddress = server
                    mgr.protocolConfiguration = proto
                    mgr.localizedDescription = "MASQUE"
                    mgr.isEnabled = true
                    mgr.saveToPreferences { saveErr in
                        self.prefs.async {
                            if let saveErr {
                                completion(.failure(saveErr))
                                return
                            }
                            self.cachedManager = nil
                            self.reloadManager { fresh in
                                guard let fresh else {
                                    completion(.failure(NSError(
                                        domain: "masque",
                                        code: 5,
                                        userInfo: [NSLocalizedDescriptionKey: "VPN configuration not found after save"]
                                    )))
                                    return
                                }
                                fresh.loadFromPreferences { reloadErr in
                                    self.prefs.async {
                                        if let reloadErr {
                                            completion(.failure(reloadErr))
                                        } else {
                                            self.cachedManager = fresh
                                            completion(.success(fresh))
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    private func reloadManager(completion: @escaping (NETunnelProviderManager?) -> Void) {
        NETunnelProviderManager.loadAllFromPreferences { [weak self] list, _ in
            self?.prefs.async {
                let match = (list ?? []).first {
                    ($0.protocolConfiguration as? NETunnelProviderProtocol)?.providerBundleIdentifier
                        == VPNManager.providerBundleID
                }
                self?.cachedManager = match
                completion(match)
            }
        }
    }

    private func applyConnectionStatus(from connection: NEVPNConnection?) {
        if let connection {
            publish(status: connection.status)
            return
        }
        prefs.async { [weak self] in
            let s = self?.cachedManager?.connection.status ?? .invalid
            DispatchQueue.main.async { self?.publish(status: s) }
        }
    }

    private func publish(status: NEVPNStatus) {
        switch status {
        case .connected:
            connected = true
            setBusy(false)
            statusText = "Status: VPN active"
        case .connecting, .reasserting:
            connected = false
            setBusy(true)
            statusText = "Status: connecting"
        case .disconnecting:
            connected = false
            setBusy(true)
            statusText = "Status: disconnecting"
        default:
            connected = false
            setBusy(false)
            pingText = "Ping: —"
            let tunnelErr = AppGroup.defaults.string(forKey: AppGroup.defaultsLastError) ?? ""
            if !tunnelErr.isEmpty {
                lastError = tunnelErr
            }
            refreshProfileStatus()
        }
    }

    private func refreshPing() {
        guard connected else {
            let tunnelErr = AppGroup.defaults.string(forKey: AppGroup.defaultsLastError) ?? ""
            if !tunnelErr.isEmpty, lastError != tunnelErr {
                lastError = tunnelErr
            }
            // Show how far the extension got: starting / opening UDP / dialing QUIC.
            if busy, tunnelErr.isEmpty {
                let stage = AppGroup.defaults.string(forKey: AppGroup.defaultsStatus) ?? ""
                if !stage.isEmpty { statusText = "Status: \(stage)" }
            }
            return
        }
        let msg = AppGroup.defaults.string(forKey: AppGroup.defaultsStatus) ?? ""
        if !msg.isEmpty { statusText = "Status: \(msg)" }
        let ms = AppGroup.defaults.integer(forKey: AppGroup.defaultsPing)
        if ms > 0 {
            pingText = "Ping: \(ms) ms"
        }
    }
}
