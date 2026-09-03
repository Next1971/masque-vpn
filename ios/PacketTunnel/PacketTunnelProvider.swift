import Darwin
import Foundation
import NetworkExtension
import Mobile

final class PacketTunnelProvider: NEPacketTunnelProvider {
    private var tunnel: MobileTunnel?
    private var goCallback: GoCallback?
    private var pingTimer: DispatchSourceTimer?
    private let writeQueue = DispatchQueue(label: "com.next1971.masque.tun-write")
    private let workQueue = DispatchQueue(label: "com.next1971.masque.provider")
    private var startCompleted = false

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        workQueue.async { [weak self] in
            self?.startTunnelLocked(completionHandler: completionHandler)
        }
    }

    private func finishStart(_ completionHandler: @escaping (Error?) -> Void, _ error: Error?) {
        // Must run on workQueue.
        guard !startCompleted else { return }
        startCompleted = true
        completionHandler(error)
    }

    private func startTunnelLocked(completionHandler: @escaping (Error?) -> Void) {
        startCompleted = false

        guard let profile = ProfileStore.load() else {
            finishStart(completionHandler, NSError(
                domain: "masque",
                code: 1,
                userInfo: [NSLocalizedDescriptionKey: "profile not configured"]
            ))
            return
        }

        let cfg = MobileConfig()
        cfg.server = profile.server
        cfg.serverName = profile.serverName
        cfg.caPath = profile.caPath
        cfg.certPath = profile.certPath
        cfg.keyPath = profile.keyPath
        cfg.mtu = profile.mtu

        let cb = GoCallback(owner: self)
        goCallback = cb
        var dialErr: NSError?
        guard let t = MobileDial(cfg, cb, &dialErr) else {
            goCallback = nil
            finishStart(completionHandler, dialErr)
            return
        }
        tunnel = t

        let settings = self.networkSettings(for: t, profile: profile)

        setTunnelNetworkSettings(settings) { [weak self] err in
            guard let self else {
                completionHandler(err)
                return
            }
            self.workQueue.async {
                if let err {
                    t.stop()
                    self.tunnel = nil
                    self.goCallback = nil
                    self.finishStart(completionHandler, err)
                    return
                }
                do {
                    try t.startPacketBridge()
                } catch {
                    t.stop()
                    self.tunnel = nil
                    self.goCallback = nil
                    self.finishStart(completionHandler, error)
                    return
                }
                self.pumpFromDevice()
                self.pumpToDevice()
                self.startPingTimer()
                self.publishStatus("VPN active")
                self.finishStart(completionHandler, nil)
            }
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        workQueue.async { [weak self] in
            guard let self else {
                completionHandler()
                return
            }
            self.pingTimer?.cancel()
            self.pingTimer = nil
            self.tunnel?.stop()
            self.tunnel = nil
            self.goCallback = nil
            AppGroup.defaults.set(0, forKey: AppGroup.defaultsPing)
            AppGroup.defaults.set("Disconnected", forKey: AppGroup.defaultsStatus)
            completionHandler()
        }
    }

    fileprivate func handleGoStatus(_ msg: String) {
        workQueue.async { [weak self] in
            guard let self else { return }
            if msg == "assigned-ip-changed" {
                self.cancelTunnelWithError(NSError(
                    domain: "masque",
                    code: 2,
                    userInfo: [NSLocalizedDescriptionKey: "assigned IP changed"]
                ))
                return
            }
            self.publishStatus(msg)
        }
    }

    fileprivate func handleGoError(_ msg: String) {
        workQueue.async { [weak self] in
            self?.cancelTunnelWithError(NSError(
                domain: "masque",
                code: 3,
                userInfo: [NSLocalizedDescriptionKey: msg]
            ))
        }
    }

    private func publishStatus(_ msg: String) {
        AppGroup.defaults.set(msg, forKey: AppGroup.defaultsStatus)
        if let ms = tunnel?.rttMillis(), ms > 0 {
            AppGroup.defaults.set(Int(ms), forKey: AppGroup.defaultsPing)
        }
    }

    private func startPingTimer() {
        let timer = DispatchSource.makeTimerSource(queue: writeQueue)
        timer.schedule(deadline: .now() + 2, repeating: 2)
        timer.setEventHandler { [weak self] in
            guard let self, let t = self.tunnel else { return }
            let ms = t.rttMillis()
            if ms > 0 {
                AppGroup.defaults.set(Int(ms), forKey: AppGroup.defaultsPing)
            }
        }
        timer.resume()
        pingTimer = timer
    }

    private func networkSettings(for tunnel: MobileTunnel, profile: MasqueProfile) -> NEPacketTunnelNetworkSettings {
        var v4 = tunnel.assignedAddr() ?? ""
        if v4.isEmpty { v4 = "10.8.0.254" }
        let remoteRaw = tunnel.serverIPv4()
        let remote = remoteRaw.isEmpty ? "127.0.0.1" : remoteRaw
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: remote)
        settings.mtu = NSNumber(value: profile.mtu)

        let ipv4 = NEIPv4Settings(addresses: [v4], subnetMasks: ["255.255.255.0"])
        ipv4.includedRoutes = [NEIPv4Route.default()]
        if remote != "127.0.0.1" {
            ipv4.excludedRoutes = [NEIPv4Route(destinationAddress: remote, subnetMask: "255.255.255.255")]
        }
        settings.ipv4Settings = ipv4

        let v6addr = tunnel.assignedAddrV6() ?? ""
        let ipv6: NEIPv6Settings
        if !v6addr.isEmpty {
            ipv6 = NEIPv6Settings(addresses: [v6addr], networkPrefixLengths: [64])
        } else {
            ipv6 = NEIPv6Settings(addresses: ["fd00::1"], networkPrefixLengths: [128])
        }
        ipv6.includedRoutes = [NEIPv6Route.default()]
        settings.ipv6Settings = ipv6

        var dns = [profile.dns]
        if profile.dns != "8.8.8.8" { dns.append("8.8.8.8") }
        settings.dnsSettings = NEDNSSettings(servers: dns)
        return settings
    }

    private func pumpFromDevice() {
        packetFlow.readPackets { [weak self] packets, _ in
            guard let self, let tunnel = self.tunnel else { return }
            for pkt in packets {
                _ = try? tunnel.writePacket(pkt)
            }
            self.pumpFromDevice()
        }
    }

    private func pumpToDevice() {
        writeQueue.async { [weak self] in
            guard let self else { return }
            while true {
                guard let tunnel = self.tunnel else { return }
                guard let pkt = tunnel.readPacket(), !pkt.isEmpty else { return }
                let proto = Self.ipProtocol(pkt)
                self.packetFlow.writePackets([pkt], withProtocols: [proto])
            }
        }
    }

    private static func ipProtocol(_ pkt: Data) -> NSNumber {
        guard let first = pkt.first else { return NSNumber(value: AF_INET) }
        if first >> 4 == 6 {
            return NSNumber(value: AF_INET6)
        }
        return NSNumber(value: AF_INET)
    }
}

private final class GoCallback: MobileCallback {
    weak var owner: PacketTunnelProvider?
    init(owner: PacketTunnelProvider) { super.init(); self.owner = owner }

    override func onStatus(_ msg: String?) {
        guard let msg else { return }
        owner?.handleGoStatus(msg)
    }

    override func onError(_ msg: String?) {
        guard let msg else { return }
        owner?.handleGoError(msg)
    }
}
