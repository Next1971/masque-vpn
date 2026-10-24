import Combine
import Foundation
import NetworkExtension

final class VPNManager: ObservableObject {
    static let providerBundleID = "com.next1971.masque.packet-tunnel"

    @Published var statusText = "Status: profile not configured"
    @Published var pingText = "Ping: —"
    @Published var connected = false
    @Published var busy = false
    @Published var mtuText = "1340"
    @Published var transport = "auto"
    @Published var killSwitch = false
    @Published var trafficText = "Today: 0.0 MB\nMonth: 0.0 MB"
    @Published var lastError: String?

    /// User tapped Disconnect or cancel. Stops the kill-switch retry.
    private var userStop = false
    private var killSwitchRetryPending = false
    private var killSwitchRetryAttempt = 0

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
        if busy { return "Stop" }
        return connected ? "Disconnect" : "Connect"
    }

    func refreshProfileStatus() {
        if ProfileStore.isConfigured() {
            if !connected && !busy {
                // Keep a failure on screen: "profile ready" used to wipe the
                // reason the tunnel just died.
                let tunnelErr = Self.displayError(AppGroup.defaults.string(forKey: AppGroup.defaultsLastError) ?? "")
                statusText = tunnelErr.isEmpty ? "Status: profile ready" : "Status: \(tunnelErr)"
            }
        } else {
            statusText = "Status: profile not configured"
        }
        let stored = AppGroup.defaults.object(forKey: AppGroup.defaultsMTU) as? Int ?? 1340
        mtuText = String(clampMTU(stored))
        let mode = AppGroup.defaults.string(forKey: AppGroup.defaultsTransport) ?? "auto"
        transport = (mode == "udp" || mode == "tcp") ? mode : "auto"
        killSwitch = AppGroup.defaults.bool(forKey: AppGroup.defaultsKillSwitch)
        trafficText = TrafficAccount.label()
    }

    func saveSettings(transport: String, killSwitch: Bool, mtuText: String) {
        let mode = (transport == "udp" || transport == "tcp") ? transport : "auto"
        let mtu = clampMTU(Int(mtuText) ?? 1340)
        self.transport = mode
        self.killSwitch = killSwitch
        self.mtuText = String(mtu)
        ProfileStore.saveMTU(mtu)
        AppGroup.defaults.set(mode, forKey: AppGroup.defaultsTransport)
        AppGroup.defaults.set(killSwitch, forKey: AppGroup.defaultsKillSwitch)
        // Off while already disconnected: drop any block left from the last session.
        // Off while connected: the current session stays armed until Disconnect.
        if !killSwitch && !connected {
            AppGroup.defaults.set(false, forKey: AppGroup.defaultsKillSwitchArmed)
            AppGroup.defaults.set(false, forKey: AppGroup.defaultsSessionUp)
            prefs.async { [weak self] in
                self?.releaseSystemKillSwitch()
            }
        }
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

    func importText(_ text: String) {
        lastError = nil
        do {
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
            statusText = "Status: cancelling"
            beginUserStop()
            return
        }

        if connected {
            setBusy(true)
            statusText = "Status: disconnecting"
            beginUserStop()
            return
        }

        guard ProfileStore.isConfigured(), let profile = ProfileStore.load() else {
            lastError = "Import a profile first"
            return
        }
        let mtu = clampMTU(Int(mtuText) ?? 1340)
        mtuText = String(mtu)
        ProfileStore.saveMTU(mtu)

        userStop = false
        setBusy(true)
        statusText = "Status: starting…"
        let arm = AppGroup.defaults.bool(forKey: AppGroup.defaultsKillSwitch)
        AppGroup.defaults.set(arm, forKey: AppGroup.defaultsKillSwitchArmed)
        if !arm {
            AppGroup.defaults.set(false, forKey: AppGroup.defaultsSessionUp)
        }

        prefs.async { [weak self] in
            self?.startTunnel(server: profile.server, attempt: 0)
        }
    }

    /// Disconnect clears the system block before stop, or a stopped profile
    /// with includeAllNetworks still set leaves the phone with no internet.
    private func beginUserStop() {
        userStop = true
        killSwitchRetryPending = false
        killSwitchRetryAttempt = 0
        AppGroup.defaults.set(false, forKey: AppGroup.defaultsKillSwitchArmed)
        AppGroup.defaults.set(false, forKey: AppGroup.defaultsSessionUp)
        prefs.async { [weak self] in
            self?.stopReleasingKillSwitch()
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
        busyWatchdog = Timer.scheduledTimer(withTimeInterval: 20, repeats: false) { [weak self] _ in
            guard let self, self.busy, !self.connected else { return }
            self.busy = false
            let tunnelErr = Self.displayError(AppGroup.defaults.string(forKey: AppGroup.defaultsLastError) ?? "")
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
                if self.userStop {
                    mgr.connection.stopVPNTunnel()
                    DispatchQueue.main.async {
                        self.setBusy(false)
                        self.refreshProfileStatus()
                    }
                    return
                }
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
                    // Armed only for this connect. Disconnect clears it again
                    // before stopVPNTunnel, so a stopped profile does not keep
                    // blocking the phone. No IPv6 default route is added.
                    let armed = AppGroup.defaults.bool(forKey: AppGroup.defaultsKillSwitchArmed)
                    proto.includeAllNetworks = armed
                    if #available(iOS 16.4, *) {
                        proto.excludeAPNs = armed
                        proto.excludeCellularServices = armed
                    }
                    mgr.protocolConfiguration = proto
                    mgr.localizedDescription = "MASQUE"
                    mgr.isEnabled = true
                    // Bring the tunnel back if iOS jetsams the extension while
                    // other apps are open. Disconnect turns this off first.
                    let always = NEOnDemandRuleConnect()
                    always.interfaceTypeMatch = .any
                    mgr.onDemandRules = [always]
                    mgr.isOnDemandEnabled = true
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
            userStop = false
            lastError = nil
            AppGroup.defaults.removeObject(forKey: AppGroup.defaultsLastError)
            killSwitchRetryAttempt = 0
            killSwitchRetryPending = false
            let stage = AppGroup.defaults.string(forKey: AppGroup.defaultsStatus) ?? ""
            if stage.hasPrefix("VPN active") {
                setBusy(false)
                statusText = "Status: VPN active"
            } else {
                setBusy(true)
                statusText = "Status: \(stage.isEmpty ? "connecting" : stage)"
            }
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
            pingText = "Ping: —"
            if userStop {
                userStop = false
                killSwitchRetryPending = false
                setBusy(false)
                refreshProfileStatus()
                return
            }
            setBusy(false)
            if !AppGroup.defaults.bool(forKey: AppGroup.defaultsKillSwitch) {
                AppGroup.defaults.set(false, forKey: AppGroup.defaultsKillSwitchArmed)
                AppGroup.defaults.set(false, forKey: AppGroup.defaultsSessionUp)
                prefs.async { [weak self] in
                    self?.releaseSystemKillSwitch()
                }
            } else if considerKillSwitchRearm() {
                return
            } else {
                releaseIfNeverUp()
            }
            let tunnelErr = Self.displayError(AppGroup.defaults.string(forKey: AppGroup.defaultsLastError) ?? "")
            if !tunnelErr.isEmpty {
                lastError = tunnelErr
            }
            refreshProfileStatus()
        }
    }

    /// After a real session, a drop with the kill switch armed reconnects and
    /// leaves includeAllNetworks set, so traffic stays blocked in the gap.
    /// Does not release the block; that happens only from a disconnect.
    private func considerKillSwitchRearm() -> Bool {
        if userStop { return false }
        guard AppGroup.defaults.bool(forKey: AppGroup.defaultsKillSwitchArmed),
              AppGroup.defaults.bool(forKey: AppGroup.defaultsKillSwitch),
              AppGroup.defaults.bool(forKey: AppGroup.defaultsSessionUp),
              ProfileStore.isConfigured(),
              ProfileStore.load() != nil else { return false }
        if busy || killSwitchRetryPending { return true }
        killSwitchRetryPending = true
        busy = true
        statusText = "Status: Kill switch: traffic blocked, reconnecting"
        let delay = min(8.0, Double(killSwitchRetryAttempt + 1))
        killSwitchRetryAttempt += 1
        DispatchQueue.main.asyncAfter(deadline: .now() + delay) { [weak self] in
            guard let self else { return }
            self.killSwitchRetryPending = false
            guard !self.userStop, !self.connected else { return }
            guard AppGroup.defaults.bool(forKey: AppGroup.defaultsKillSwitch),
                  AppGroup.defaults.bool(forKey: AppGroup.defaultsKillSwitchArmed),
                  AppGroup.defaults.bool(forKey: AppGroup.defaultsSessionUp),
                  let profile = ProfileStore.load() else {
                self.busy = false
                self.refreshProfileStatus()
                return
            }
            self.busy = true
            self.statusText = "Status: Kill switch: traffic blocked, reconnecting"
            self.prefs.async {
                self.startTunnel(server: profile.server, attempt: 0)
            }
        }
        return true
    }

    /// A failed first connect must not leave includeAllNetworks set.
    private func releaseIfNeverUp() {
        guard AppGroup.defaults.bool(forKey: AppGroup.defaultsKillSwitchArmed) else { return }
        guard !AppGroup.defaults.bool(forKey: AppGroup.defaultsSessionUp) else { return }
        AppGroup.defaults.set(false, forKey: AppGroup.defaultsKillSwitchArmed)
        prefs.async { [weak self] in
            self?.releaseSystemKillSwitch()
        }
    }

    private func stopReleasingKillSwitch() {
        reloadManager { [weak self] mgr in
            guard let self else { return }
            guard let mgr else {
                DispatchQueue.main.async {
                    self.setBusy(false)
                    self.refreshProfileStatus()
                }
                return
            }
            var stopped = false
            let stopNow: () -> Void = {
                guard !stopped else { return }
                stopped = true
                mgr.connection.stopVPNTunnel()
                DispatchQueue.main.async {
                    self.setBusy(false)
                    self.connected = false
                    self.refreshProfileStatus()
                }
            }
            mgr.loadFromPreferences { _ in
                self.prefs.async {
                    self.setIncludeAllNetworks(false, on: mgr)
                    mgr.isOnDemandEnabled = false
                    mgr.saveToPreferences { _ in
                        self.prefs.async { stopNow() }
                    }
                }
            }
            // saveToPreferences used to hang Disconnect. Stop anyway.
            DispatchQueue.global().asyncAfter(deadline: .now() + 2.5) {
                self.prefs.async { stopNow() }
            }
        }
    }

    /// Transient UDP/DNS noise must not replace the Connect button with a wall of text.
    private static func displayError(_ raw: String) -> String {
        let m = raw.lowercased()
        if m.contains("udp write failed") { return "" }
        if m.contains("socket is not connected") || m.contains("сокет не подключен") { return "" }
        if raw.hasPrefix("could not resolve") {
            return "Could not resolve the VPN server. Check mobile data / DNS and try again."
        }
        return raw
    }

    /// Clear a leftover system block. No save when the flag is already off.
    private func releaseSystemKillSwitch() {
        reloadManager { [weak self] mgr in
            guard let self, let mgr else { return }
            mgr.loadFromPreferences { _ in
                self.prefs.async {
                    guard let proto = mgr.protocolConfiguration as? NETunnelProviderProtocol,
                          proto.includeAllNetworks else { return }
                    self.setIncludeAllNetworks(false, on: mgr)
                    mgr.saveToPreferences { _ in }
                }
            }
        }
    }

    private func setIncludeAllNetworks(_ on: Bool, on mgr: NETunnelProviderManager) {
        guard let proto = mgr.protocolConfiguration as? NETunnelProviderProtocol else { return }
        proto.includeAllNetworks = on
        if #available(iOS 16.4, *) {
            proto.excludeAPNs = on
            proto.excludeCellularServices = on
        }
        mgr.protocolConfiguration = proto
    }

    private func refreshPing() {
        trafficText = TrafficAccount.label()
        guard connected else {
            if considerKillSwitchRearm() { return }
            let tunnelErr = Self.displayError(AppGroup.defaults.string(forKey: AppGroup.defaultsLastError) ?? "")
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
