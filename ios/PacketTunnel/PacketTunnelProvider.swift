import Darwin
import Foundation
import NetworkExtension
import Mobile

final class PacketTunnelProvider: NEPacketTunnelProvider {
    private var tunnel: MobileTunnel?
    private var pingTimer: DispatchSourceTimer?
    private let writeQueue = DispatchQueue(label: "com.next1971.masque.tun-write")
    private var readingFromGo = false

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        guard let profile = ProfileStore.load() else {
            completionHandler(NSError(domain: "masque", code: 1, userInfo: [NSLocalizedDescriptionKey: "profile not configured"]))
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
        var dialErr: NSError?
        guard let t = MobileDial(cfg, cb, &dialErr) else {
            completionHandler(dialErr)
            return
        }
        tunnel = t

        let settings = self.networkSettings(for: t, profile: profile)

        setTunnelNetworkSettings(settings) { [weak self] err in
            guard let self else {
                completionHandler(err)
                return
            }
            if let err {
                t.stop()
                self.tunnel = nil
                completionHandler(err)
                return
            }
            t.startPacketBridge()
            self.pumpFromDevice()
            self.pumpToDevice()
            self.startPingTimer()
            self.publishStatus("VPN active")
            completionHandler(nil)
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        pingTimer?.cancel()
        pingTimer = nil
        tunnel?.stop()
        tunnel = nil
        readingFromGo = false
        AppGroup.defaults.set(0, forKey: AppGroup.defaultsPing)
        AppGroup.defaults.set("Disconnected", forKey: AppGroup.defaultsStatus)
        completionHandler()
    }

    fileprivate func handleGoStatus(_ msg: String) {
        if msg == "assigned-ip-changed" {
            cancelTunnelWithError(NSError(domain: "masque", code: 2, userInfo: [NSLocalizedDescriptionKey: "assigned IP changed"]))
            return
        }
        publishStatus(msg)
    }

    fileprivate func handleGoError(_ msg: String) {
        cancelTunnelWithError(NSError(domain: "masque", code: 3, userInfo: [NSLocalizedDescriptionKey: msg]))
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
        guard !readingFromGo else { return }
        readingFromGo = true
        writeQueue.async { [weak self] in
            guard let self else { return }
            while let tunnel = self.tunnel {
                guard let pkt = tunnel.readPacket(), !pkt.isEmpty else { break }
                let proto = Self.ipProtocol(pkt)
                self.packetFlow.writePackets([pkt], withProtocols: [proto])
            }
            self.readingFromGo = false
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
