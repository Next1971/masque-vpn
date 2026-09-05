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
    private let dialQueue = DispatchQueue(label: "com.next1971.masque.dial")
    private let udpQueue = DispatchQueue(label: "com.next1971.masque.udp")
    private var startCompleted = false
    private var startHandler: ((Error?) -> Void)?

    private var udpSession: NWUDPSession?
    private var udpStateObs: NSKeyValueObservation?
    private var udpWriter: GoUDPWriter?
    private var datagramPipe: MobileDatagramPipe?
    private var udpWriteErrorLogged = false

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        workQueue.async { [weak self] in
            self?.startTunnelLocked(completionHandler: completionHandler)
        }
    }

    private func finishStart(_ error: Error?) {
        guard !startCompleted else { return }
        startCompleted = true
        let handler = startHandler
        startHandler = nil
        handler?(error)
    }

    /// Finish startTunnel on a bootstrap interface (iOS deadline), then open
    /// UDP. Build 10 opened createUDPSession in parallel with
    /// setTunnelNetworkSettings; applying routes cancelled that session in ~1s
    /// (opening UDP, never dialing QUIC, no packets on the VPS).
    private func startTunnelLocked(completionHandler: @escaping (Error?) -> Void) {
        startCompleted = false
        startHandler = completionHandler
        udpWriteErrorLogged = false
        AppGroup.defaults.removeObject(forKey: AppGroup.defaultsLastError)

        guard let profile = ProfileStore.load() else {
            finishStart(NSError(
                domain: "masque",
                code: 1,
                userInfo: [NSLocalizedDescriptionKey: "profile not configured"]
            ))
            return
        }

        let host = Self.host(of: profile.server)
        let port = Self.port(of: profile.server)
        let ip = Self.resolveIPv4(host)
        let remote = ip ?? "127.0.0.1"
        let dialServer = ip.map { "\($0):\(port)" } ?? profile.server

        guard let serverIP = ip, let portNum = Int(port), portNum > 0 else {
            let msg = ip == nil
                ? "could not resolve \(host) to IPv4"
                : "invalid server port \(port)"
            recordError(msg)
            finishStart(NSError(
                domain: "masque",
                code: 7,
                userInfo: [NSLocalizedDescriptionKey: msg]
            ))
            return
        }

        publishStatus("starting")
        let bootstrap = bootstrapSettings(remote: remote, profile: profile)
        setTunnelNetworkSettings(bootstrap) { [weak self] err in
            guard let self else {
                completionHandler(err)
                return
            }
            self.workQueue.async {
                if let err {
                    self.recordError(err.localizedDescription)
                    self.finishStart(err)
                    return
                }
                self.finishStart(nil)
                // Let the exclude-route path settle. Binding from: physicalIP:0
                // (build 11) made NWUDPSession jump to .failed immediately.
                self.workQueue.asyncAfter(deadline: .now() + 0.4) { [weak self] in
                    guard let self else { return }
                    self.publishStatus("opening UDP to \(serverIP):\(port)")
                    self.openPhysicalUDP(host: serverIP, port: port) { [weak self] udpErr in
                        guard let self else { return }
                        self.workQueue.async {
                            if let udpErr {
                                self.failAfterStart(udpErr.localizedDescription, code: 9)
                                return
                            }
                            self.attachPipeAndDial(
                                profile: profile,
                                dialServer: dialServer,
                                remote: remote,
                                serverIP: serverIP,
                                portNum: portNum
                            )
                        }
                    }
                }
            }
        }
    }

    /// Fail startTunnel if it is still open; otherwise tear the tunnel down.
    private func failAfterStart(_ msg: String, code: Int) {
        recordError(msg)
        teardownUDP()
        let err = NSError(
            domain: "masque",
            code: code,
            userInfo: [NSLocalizedDescriptionKey: msg]
        )
        if startCompleted {
            cancelTunnelWithError(err)
        } else {
            finishStart(err)
        }
    }

    private func attachPipeAndDial(
        profile: MasqueProfile,
        dialServer: String,
        remote: String,
        serverIP: String,
        portNum: Int
    ) {
        guard let session = udpSession else {
            failAfterStart("UDP session missing after ready", code: 8)
            return
        }

        let writer = GoUDPWriter(session: session, queue: udpQueue, owner: self)
        udpWriter = writer
        var pipeErr: NSError?
        guard let pipe = MobileNewDatagramPipe(writer, serverIP, portNum, &pipeErr) else {
            failAfterStart(pipeErr?.localizedDescription ?? "UDP pipe failed", code: 8)
            return
        }
        datagramPipe = pipe
        session.setReadHandler({ [weak self] datagrams, readErr in
            if let readErr {
                self?.workQueue.async {
                    // A settings update cancels in-flight reads. Do not overwrite
                    // a live CONNECT-IP status with that cancellation.
                    guard self?.tunnel == nil else { return }
                    self?.recordError("UDP read: \(readErr.localizedDescription)")
                }
            }
            guard let pipe = self?.datagramPipe else { return }
            for d in datagrams ?? [] {
                pipe.deliver(d)
            }
        }, maxDatagrams: 32)

        publishStatus("dialing QUIC")
        dialQueue.async { [weak self] in
            self?.dialThenApplySettings(
                profile: profile,
                dialServer: dialServer,
                remote: remote,
                pipe: pipe
            )
        }
    }

    /// Dummy host-route only. Build 13 used this, then applied the real default
    /// after Dial. Build 14 put the default route on *this* first apply: first
    /// connect worked, then the session died in minutes and later Connect never
    /// reached the VPS (UDP already sitting on the capture path).
    private func bootstrapSettings(remote: String, profile: MasqueProfile) -> NEPacketTunnelNetworkSettings {
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: remote)
        settings.mtu = NSNumber(value: profile.mtu)
        let ipv4 = NEIPv4Settings(addresses: [Self.placeholderIPv4], subnetMasks: ["255.255.255.255"])
        ipv4.includedRoutes = [NEIPv4Route(destinationAddress: "198.18.0.1", subnetMask: "255.255.255.255")]
        if remote != "127.0.0.1" {
            ipv4.excludedRoutes = [NEIPv4Route(destinationAddress: remote, subnetMask: "255.255.255.255")]
        }
        settings.ipv4Settings = ipv4
        let ipv6 = NEIPv6Settings(addresses: ["fd00::1"], networkPrefixLengths: [128])
        ipv6.includedRoutes = []
        settings.ipv6Settings = ipv6
        return settings
    }

    private func openPhysicalUDP(host: String, port: String, completion: @escaping (Error?) -> Void) {
        teardownUDP()
        // from: nil — the Packet Tunnel API picks the physical path using
        // excludedRoutes. from: "<wifi>:0" is rejected and the session fails.
        let endpoint = NWHostEndpoint(hostname: host, port: port)
        let session = createUDPSession(to: endpoint, from: nil)
        udpSession = session

        var finished = false
        let finish: (Error?) -> Void = { [weak self] err in
            self?.workQueue.async {
                guard !finished else { return }
                finished = true
                completion(err)
            }
        }

        udpStateObs = session.observe(\.state, options: [.new]) { [weak self] sess, _ in
            self?.publishStatus("UDP state \(sess.state.rawValue)")
            switch sess.state {
            case .ready:
                finish(nil)
            case .failed:
                let msg = Self.udpErrorText(sess)
                finish(NSError(
                    domain: "masque",
                    code: 9,
                    userInfo: [NSLocalizedDescriptionKey: msg]
                ))
                self?.workQueue.async {
                    guard let self, self.startCompleted, self.tunnel != nil else { return }
                    self.failAfterStart(msg, code: 9)
                }
            case .cancelled:
                finish(NSError(
                    domain: "masque",
                    code: 9,
                    userInfo: [NSLocalizedDescriptionKey: "UDP session cancelled"]
                ))
                self?.workQueue.async {
                    guard let self, self.startCompleted, self.tunnel != nil else { return }
                    self.failAfterStart("UDP session cancelled", code: 9)
                }
            default:
                break
            }
        }

        switch session.state {
        case .ready:
            finish(nil)
        case .failed:
            finish(NSError(
                domain: "masque",
                code: 9,
                userInfo: [NSLocalizedDescriptionKey: Self.udpErrorText(session)]
            ))
        default:
            break
        }

        workQueue.asyncAfter(deadline: .now() + 8) { [weak self] in
            guard let self, let session = self.udpSession else { return }
            if session.state != .ready {
                finish(NSError(
                    domain: "masque",
                    code: 9,
                    userInfo: [NSLocalizedDescriptionKey: "UDP session not ready (state \(session.state.rawValue))"]
                ))
            }
        }
    }

    /// NWUDPSession exposes no error, so report the path status and whether the
    /// endpoint resolved — enough to tell "no route" from "DNS/endpoint bad".
    private static func udpErrorText(_ session: NWUDPSession) -> String {
        let pathStatus = session.currentPath.map { "\($0.status.rawValue)" } ?? "none"
        let target = (session.resolvedEndpoint as? NWHostEndpoint)
            .map { "\($0.hostname):\($0.port)" } ?? "unresolved"
        return "UDP session failed (path \(pathStatus), \(target))"
    }

    private func dialThenApplySettings(
        profile: MasqueProfile,
        dialServer: String,
        remote: String,
        pipe: MobileDatagramPipe
    ) {
        let cfg = MobileConfig()
        cfg.server = dialServer
        cfg.serverName = profile.serverName
        cfg.caPath = profile.caPath
        cfg.certPath = profile.certPath
        cfg.keyPath = profile.keyPath
        cfg.mtu = profile.mtu
        cfg.bindInterface = ""

        let cb = GoCallback(owner: self)
        goCallback = cb
        var dialErr: NSError?
        guard let t = MobileDialWithPipe(cfg, cb, pipe, &dialErr) else {
            goCallback = nil
            let msg = dialErr?.localizedDescription ?? "QUIC dial failed"
            workQueue.async { [weak self] in
                self?.failAfterStart(msg, code: 6)
            }
            return
        }
        tunnel = t

        // Same as build 13: apply the real default route after Dial. A cancelled
        // read during this apply is noisy but the session stayed up (~8 Mbit).
        let settings = networkSettings(for: t, profile: profile, fallbackRemote: remote)
        setTunnelNetworkSettings(settings) { [weak self] err in
            guard let self else { return }
            self.workQueue.async {
                if let err {
                    t.stop()
                    self.tunnel = nil
                    self.goCallback = nil
                    self.failAfterStart(err.localizedDescription, code: 10)
                    return
                }
                self.rearmUDPRead(pipe: pipe)
                do {
                    try t.startPacketBridge()
                } catch {
                    t.stop()
                    self.tunnel = nil
                    self.goCallback = nil
                    self.failAfterStart(error.localizedDescription, code: 11)
                    return
                }
                self.pumpFromDevice()
                self.pumpToDevice()
                self.startPingTimer()
                self.publishStatus("VPN active")
            }
        }
    }

    /// setReadHandler stops after a cancelled read (settings apply). Arm it again.
    private func rearmUDPRead(pipe: MobileDatagramPipe) {
        guard let session = udpSession else { return }
        session.setReadHandler({ [weak self] datagrams, readErr in
            if let readErr {
                let text = readErr.localizedDescription.lowercased()
                if text.contains("cancel") || text.contains("отмен") {
                    return
                }
                self?.workQueue.async {
                    guard self?.tunnel == nil else { return }
                    self?.recordError("UDP read: \(readErr.localizedDescription)")
                }
            }
            for d in datagrams ?? [] {
                pipe.deliver(d)
            }
        }, maxDatagrams: 32)
    }

    private func teardownUDP() {
        udpStateObs?.invalidate()
        udpStateObs = nil
        try? datagramPipe?.close()
        datagramPipe = nil
        udpWriter = nil
        udpSession?.cancel()
        udpSession = nil
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
            self.teardownUDP()
            AppGroup.defaults.set(0, forKey: AppGroup.defaultsPing)
            AppGroup.defaults.set("Disconnected", forKey: AppGroup.defaultsStatus)
            completionHandler()
        }
    }

    /// First failed datagram send only: QUIC retransmits, so this would spam.
    fileprivate func noteUDPWriteError(_ err: Error) {
        workQueue.async { [weak self] in
            guard let self, !self.udpWriteErrorLogged else { return }
            self.udpWriteErrorLogged = true
            self.recordError("UDP write failed: \(err.localizedDescription)")
        }
    }

    fileprivate func handleGoStatus(_ msg: String) {
        workQueue.async { [weak self] in
            guard let self else { return }
            if msg == "assigned-ip-changed" {
                self.recordError("assigned IP changed")
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
            self?.recordError(msg)
            self?.cancelTunnelWithError(NSError(
                domain: "masque",
                code: 3,
                userInfo: [NSLocalizedDescriptionKey: msg]
            ))
        }
    }

    private func recordError(_ msg: String) {
        AppGroup.defaults.set(msg, forKey: AppGroup.defaultsLastError)
        AppGroup.defaults.set(msg, forKey: AppGroup.defaultsStatus)
        AppGroup.defaults.synchronize()
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

    private func networkSettings(for tunnel: MobileTunnel, profile: MasqueProfile, fallbackRemote: String) -> NEPacketTunnelNetworkSettings {
        var v4 = tunnel.assignedAddr() ?? ""
        if v4.isEmpty { v4 = "10.8.0.254" }
        let remoteRaw = tunnel.serverIPv4()
        let remote = remoteRaw.isEmpty ? fallbackRemote : remoteRaw
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

    private static let placeholderIPv4 = "10.8.0.254"

    private static func host(of server: String) -> String {
        if server.hasPrefix("["), let end = server.firstIndex(of: "]") {
            return String(server[server.index(after: server.startIndex)..<end])
        }
        if let colon = server.lastIndex(of: ":") {
            return String(server[..<colon])
        }
        return server
    }

    private static func port(of server: String) -> String {
        if server.hasPrefix("["), let end = server.firstIndex(of: "]") {
            let rest = server[server.index(after: end)...]
            if rest.first == ":" { return String(rest.dropFirst()) }
            return "443"
        }
        if let colon = server.lastIndex(of: ":") {
            let p = String(server[server.index(after: colon)...])
            if !p.isEmpty { return p }
        }
        return "443"
    }

    private static func resolveIPv4(_ host: String) -> String? {
        var hints = addrinfo()
        hints.ai_family = AF_INET
        hints.ai_socktype = SOCK_DGRAM
        var info: UnsafeMutablePointer<addrinfo>?
        let rc = getaddrinfo(host, nil, &hints, &info)
        guard rc == 0, let first = info else { return nil }
        defer { freeaddrinfo(info) }
        var buf = [CChar](repeating: 0, count: Int(NI_MAXHOST))
        guard getnameinfo(
            first.pointee.ai_addr,
            socklen_t(first.pointee.ai_addrlen),
            &buf,
            socklen_t(buf.count),
            nil,
            0,
            NI_NUMERICHOST
        ) == 0 else { return nil }
        return String(cString: buf)
    }

    private static func ipProtocol(_ pkt: Data) -> NSNumber {
        guard let first = pkt.first else { return NSNumber(value: AF_INET) }
        if first >> 4 == 6 {
            return NSNumber(value: AF_INET6)
        }
        return NSNumber(value: AF_INET)
    }
}

/// gomobile exports a Go interface as an ObjC protocol *plus* a proxy class for
/// Go-side values. Builds up to 12 subclassed the proxy class, so Go received an
/// object with no valid Go reference: WriteDatagram never reached Swift, nothing
/// went on the wire, and the extension died a few seconds into the dial.
/// Implement the protocol on a plain NSObject instead.
///
/// writeDatagram must not block: gomobile calls it on the Go QUIC thread.
private final class GoUDPWriter: NSObject, MobileDatagramWriterProtocol {
    private let session: NWUDPSession
    private let queue: DispatchQueue
    private weak var owner: PacketTunnelProvider?

    init(session: NWUDPSession, queue: DispatchQueue, owner: PacketTunnelProvider) {
        self.session = session
        self.queue = queue
        self.owner = owner
        super.init()
    }

    func writeDatagram(_ p: Data?) throws {
        guard let data = p, !data.isEmpty else { return }
        queue.async { [weak self] in
            guard let self else { return }
            self.session.writeDatagram(data) { [weak self] err in
                if let err {
                    self?.owner?.noteUDPWriteError(err)
                }
            }
        }
    }
}

private final class GoCallback: NSObject, MobileCallbackProtocol {
    weak var owner: PacketTunnelProvider?

    init(owner: PacketTunnelProvider) {
        self.owner = owner
        super.init()
    }

    func onStatus(_ msg: String?) {
        guard let msg else { return }
        owner?.handleGoStatus(msg)
    }

    func onError(_ msg: String?) {
        guard let msg else { return }
        owner?.handleGoError(msg)
    }
}
