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
    private var stopping = false
    private var reconnecting = false
    private var lastProfile: MasqueProfile?
    private var lastRemote = ""
    private var lastDialServer = ""
    private var lastServerIP = ""
    private var lastAssignedV4 = "10.8.0.254"
    private var lastAssignedV6 = ""
    private var lastPort = ""
    private var lastPortNum = 0
    private var everDialed = false
    private var routesApplied = false
    private var tearingDownUDP = false
    private var reconnectAttempt = 0
    private var pathObs: NSKeyValueObservation?
    private var lastPathKey = ""
    private var lastPhysicalIP = ""
    private var ignorePathChangesUntil = Date.distantPast

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        // A Packet Tunnel is jetsam-killed around 15 MB. Cap the Go heap before
        // anything allocates, or the process dies mid-session (9-19 min in) and
        // no Swift reconnect handler ever runs.
        MobileTuneForExtension(12)
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
        stopping = false
        reconnecting = false
        everDialed = false
        routesApplied = false
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
        let ips = Self.resolveServers(host)
        let serverIP = ips.first
        let remote = serverIP ?? "127.0.0.1"

        guard let serverIP, let portNum = Int(port), portNum > 0 else {
            let msg = ips.isEmpty
                ? "could not resolve \(host)"
                : "invalid server port \(port)"
            recordError(msg)
            finishStart(NSError(
                domain: "masque",
                code: 7,
                userInfo: [NSLocalizedDescriptionKey: msg]
            ))
            return
        }

        lastProfile = profile
        lastRemote = remote
        lastDialServer = Self.dialServer(serverIP, port)
        lastServerIP = serverIP
        lastPort = port
        lastPortNum = portNum
        lastPathKey = pathKey(defaultPath)
        ignorePathChangesUntil = Date().addingTimeInterval(4)
        observeDefaultPath()

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
                    self.connectCandidates(
                        profile: profile,
                        ips: ips,
                        port: port,
                        portNum: portNum
                    ) { msg in
                        self.failAfterStart(msg, code: 9)
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
        portNum: Int,
        onFailure: @escaping (String) -> Void
    ) {
        guard let session = udpSession else {
            onFailure("UDP session missing after ready")
            return
        }

        let writer = GoUDPWriter(session: session, queue: udpQueue, owner: self)
        udpWriter = writer
        var pipeErr: NSError?
        guard let pipe = MobileNewDatagramPipe(writer, serverIP, portNum, &pipeErr) else {
            onFailure(pipeErr?.localizedDescription ?? "UDP pipe failed")
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

        publishStatus("dialing QUIC \(serverIP):\(portNum)")
        dialQueue.async { [weak self] in
            self?.dialThenApplySettings(
                profile: profile,
                dialServer: dialServer,
                remote: remote,
                pipe: pipe,
                transport: "udp",
                onFailure: onFailure
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
        let ipv6 = NEIPv6Settings(addresses: ["fd00::1"], networkPrefixLengths: [128])
        ipv6.includedRoutes = []
        Self.excludeServer(remote, ipv4: ipv4, ipv6: ipv6)
        settings.ipv4Settings = ipv4
        settings.ipv6Settings = ipv6
        return settings
    }

    /// Wi-Fi → cellular leaves a satisfied-looking path with no local address
    /// for a few seconds. createUDPSession then goes .failed with
    /// "path 1, unresolved" and build 16 cancelled the VPN.
    private func waitForPhysicalPath(tries: Int, completion: @escaping () -> Void) {
        if stopping {
            completion()
            return
        }
        if let ip = Self.physicalIPv4() ?? Self.physicalIPv6() {
            let pathOK = defaultPath?.status == .satisfied
            // After Wi-Fi drops, en0 can keep its address for a moment while
            // createUDPSession already fails (path satisfied, endpoint unresolved).
            // Prefer a new address; after ~3s accept the same one (speedtest).
            let isNew = lastPhysicalIP.isEmpty || ip != lastPhysicalIP
            if pathOK, isNew || tries >= 6 {
                lastPhysicalIP = ip
                publishStatus("physical path \(ip)")
                workQueue.asyncAfter(deadline: .now() + 0.5, execute: completion)
                return
            }
        }
        if tries >= 40 {
            publishStatus("no physical IP yet, trying UDP anyway")
            completion()
            return
        }
        publishStatus("waiting for network…")
        workQueue.asyncAfter(deadline: .now() + 0.5) { [weak self] in
            self?.waitForPhysicalPath(tries: tries + 1, completion: completion)
        }
    }

    private func observeDefaultPath() {
        pathObs?.invalidate()
        pathObs = observe(\.defaultPath, options: [.new]) { [weak self] _, _ in
            guard let self else { return }
            self.workQueue.async {
                guard self.everDialed, self.startCompleted, !self.stopping else { return }
                guard !self.reconnecting else { return }
                guard Date() > self.ignorePathChangesUntil else { return }
                let key = self.pathKey(self.defaultPath)
                guard key != self.lastPathKey else { return }
                self.lastPathKey = key
                // Safari flips isExpensive and used to replace a live QUIC
                // session (server: H3_NO_ERROR in the same second). A real
                // Wi-Fi↔cellular loss still fails the UDP session, which
                // reconnects on its own.
                if self.tunnel != nil {
                    self.publishStatus("path changed, tunnel still live")
                    return
                }
                self.scheduleReconnect("network path changed")
            }
        }
    }

    private func pathKey(_ path: NetworkExtension.NWPath?) -> String {
        guard let path else { return "nil" }
        // Status only. isExpensive flips when another app opens and is not a
        // new network. Interface-type APIs need Network.NWPath, which is
        // ambiguous next to NetworkExtension.NWPath on Xcode 26.
        return "\(path.status.rawValue)"
    }

    /// First non-tunnel IPv4. utun/ipsec are the VPN itself; 10.8.0.254 is ours.
    private static func physicalIPv4() -> String? {
        var ifaddr: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&ifaddr) == 0, let first = ifaddr else { return nil }
        defer { freeifaddrs(ifaddr) }
        var ptr: UnsafeMutablePointer<ifaddrs>? = first
        while let cur = ptr {
            let flags = Int32(cur.pointee.ifa_flags)
            let name = String(cString: cur.pointee.ifa_name)
            let skip = name.hasPrefix("utun") || name.hasPrefix("ipsec")
                || name.hasPrefix("awdl") || name.hasPrefix("llw")
                || name.hasPrefix("p2p")
            if !skip, flags & IFF_UP != 0, flags & IFF_LOOPBACK == 0,
               let addr = cur.pointee.ifa_addr, addr.pointee.sa_family == sa_family_t(AF_INET) {
                var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
                let len = socklen_t(addr.pointee.sa_len)
                if getnameinfo(addr, len, &host, socklen_t(host.count), nil, 0, NI_NUMERICHOST) == 0 {
                    let ip = String(cString: host)
                    if ip != "10.8.0.254", !ip.hasPrefix("169.254") {
                        return ip
                    }
                }
            }
            ptr = cur.pointee.ifa_next
        }
        return nil
    }

    /// Global IPv6 on a physical interface. Link-local and the tunnel are skipped.
    private static func physicalIPv6() -> String? {
        var ifaddr: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&ifaddr) == 0, let first = ifaddr else { return nil }
        defer { freeifaddrs(ifaddr) }
        var ptr: UnsafeMutablePointer<ifaddrs>? = first
        while let cur = ptr {
            let flags = Int32(cur.pointee.ifa_flags)
            let name = String(cString: cur.pointee.ifa_name)
            let skip = name.hasPrefix("utun") || name.hasPrefix("ipsec")
                || name.hasPrefix("awdl") || name.hasPrefix("llw")
                || name.hasPrefix("p2p")
            if !skip, flags & IFF_UP != 0, flags & IFF_LOOPBACK == 0,
               let addr = cur.pointee.ifa_addr, addr.pointee.sa_family == sa_family_t(AF_INET6) {
                var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
                let len = socklen_t(addr.pointee.sa_len)
                if getnameinfo(addr, len, &host, socklen_t(host.count), nil, 0, NI_NUMERICHOST) == 0 {
                    let ip = String(cString: host).split(separator: "%", maxSplits: 1).first.map(String.init) ?? ""
                    if !ip.hasPrefix("fe80"), ip != "::1", !ip.isEmpty {
                        return ip
                    }
                }
            }
            ptr = cur.pointee.ifa_next
        }
        return nil
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
                self?.lastPhysicalIP = Self.physicalIPv4() ?? self?.lastPhysicalIP ?? ""
                finish(nil)
            case .failed:
                let msg = Self.udpErrorText(sess)
                finish(NSError(
                    domain: "masque",
                    code: 9,
                    userInfo: [NSLocalizedDescriptionKey: msg]
                ))
                // Handshake failures are retried by the openPhysicalUDP
                // completion. After the tunnel is up, a speedtest burst can
                // cancel this session — reconnect instead of killing the VPN.
                if self?.tearingDownUDP != true, self?.reconnecting != true {
                    self?.scheduleReconnect(msg)
                }
            case .cancelled:
                finish(NSError(
                    domain: "masque",
                    code: 9,
                    userInfo: [NSLocalizedDescriptionKey: "UDP session cancelled"]
                ))
                if self?.tearingDownUDP != true, self?.reconnecting != true {
                    self?.scheduleReconnect("UDP session cancelled")
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
        let local = physicalIPv4() ?? "no-phys-ip"
        return "UDP session failed (path \(pathStatus), \(target), \(local))"
    }

    /// "auto" tries each UDP endpoint, then TCP. "udp" stays on QUIC. "tcp" skips UDP.
    private func transportMode() -> String {
        let raw = (AppGroup.defaults.string(forKey: AppGroup.defaultsTransport) ?? "auto").lowercased()
        if raw == "udp" || raw == "tcp" { return raw }
        return "auto"
    }

    private struct UDPTarget {
        var ip: String
        var port: String
        var portNum: Int
    }

    /// Each address, profile port then alt_port. IPv6 addresses come first.
    private func udpTargets(ips: [String], port: String, portNum: Int, altPort: Int) -> [UDPTarget] {
        var ports: [(String, Int)] = [(port, portNum)]
        if altPort >= 1, altPort <= 65535, altPort != portNum {
            ports.append((String(altPort), altPort))
        }
        var out: [UDPTarget] = []
        for ip in ips {
            for item in ports {
                out.append(UDPTarget(ip: ip, port: item.0, portNum: item.1))
            }
        }
        return out
    }

    /// TCP skips UDP. Otherwise each UDP endpoint is its own session.
    /// Automatic mode opens TCP on each address (profile port and alt_port) only after UDP fails.
    private func connectCandidates(
        profile: MasqueProfile,
        ips: [String],
        port: String,
        portNum: Int,
        onFailure: @escaping (String) -> Void
    ) {
        if stopping {
            onFailure("stopped")
            return
        }
        let ordered = Self.prefer(lastServerIP, in: ips)
        if transportMode() == "tcp" {
            tryTCP(
                profile: profile,
                ips: ordered,
                port: port,
                index: 0,
                lastError: "TCP did not answer",
                onFailure: onFailure
            )
            return
        }
        let targets = udpTargets(ips: ordered, port: port, portNum: portNum, altPort: profile.altPort)
        tryUDP(
            profile: profile,
            targets: targets,
            port: port,
            index: 0,
            lastError: "UDP did not answer",
            onFailure: onFailure
        )
    }

    private func tryUDP(
        profile: MasqueProfile,
        targets: [UDPTarget],
        port: String,
        index: Int,
        lastError: String,
        onFailure: @escaping (String) -> Void
    ) {
        if stopping {
            onFailure("stopped")
            return
        }
        if index >= targets.count {
            if transportMode() == "auto" {
                var seen = Set<String>()
                var ips: [String] = []
                for target in targets where seen.insert(target.ip).inserted {
                    ips.append(target.ip)
                }
                tryTCP(
                    profile: profile,
                    ips: ips,
                    port: port,
                    index: 0,
                    lastError: lastError,
                    onFailure: onFailure
                )
                return
            }
            onFailure(lastError)
            return
        }
        let target = targets[index]
        ensureExcluded(target.ip, profile: profile) { [weak self] in
            guard let self else { return }
            self.publishStatus("opening UDP to \(target.ip):\(target.port)")
            self.openPhysicalUDP(host: target.ip, port: target.port) { [weak self] udpErr in
                guard let self else { return }
                self.workQueue.async {
                    if let udpErr {
                        self.tryUDP(
                            profile: profile,
                            targets: targets,
                            port: port,
                            index: index + 1,
                            lastError: udpErr.localizedDescription,
                            onFailure: onFailure
                        )
                        return
                    }
                    let dial = Self.dialServer(target.ip, target.port)
                    self.attachPipeAndDial(
                        profile: profile,
                        dialServer: dial,
                        remote: target.ip,
                        serverIP: target.ip,
                        portNum: target.portNum
                    ) { msg in
                        self.abandonDialAttempt()
                        self.tryUDP(
                            profile: profile,
                            targets: targets,
                            port: port,
                            index: index + 1,
                            lastError: msg,
                            onFailure: onFailure
                        )
                    }
                }
            }
        }
    }

    /// One address at a time. Go then tries that address on the profile port and alt_port.
    private func tryTCP(
        profile: MasqueProfile,
        ips: [String],
        port: String,
        index: Int,
        lastError: String,
        onFailure: @escaping (String) -> Void
    ) {
        if stopping {
            onFailure("stopped")
            return
        }
        if index >= ips.count {
            onFailure(lastError)
            return
        }
        let ip = ips[index]
        ensureExcluded(ip, profile: profile) { [weak self] in
            guard let self else { return }
            self.publishStatus("opening TCP to \(ip):\(port)")
            let dial = Self.dialServer(ip, port)
            self.dialQueue.async {
                self.dialThenApplySettings(
                    profile: profile,
                    dialServer: dial,
                    remote: ip,
                    pipe: nil,
                    transport: "tcp",
                    onFailure: { msg in
                        self.tryTCP(
                            profile: profile,
                            ips: ips,
                            port: port,
                            index: index + 1,
                            lastError: msg,
                            onFailure: onFailure
                        )
                    }
                )
            }
        }
    }

    /// Once the default route is installed, the server address has to stay
    /// excluded or the next dial is captured by the tunnel.
    private func ensureExcluded(_ ip: String, profile: MasqueProfile, completion: @escaping () -> Void) {
        if !routesApplied || ip.isEmpty || ip == lastRemote {
            completion()
            return
        }
        lastRemote = ip
        let settings = routeSettings(v4: lastAssignedV4, v6: lastAssignedV6, remote: ip, profile: profile)
        setTunnelNetworkSettings(settings) { [weak self] _ in
            self?.workQueue.async { completion() }
        }
    }

    /// Drop a failed QUIC attempt so the next port can open its own session.
    private func abandonDialAttempt() {
        tunnel?.stop()
        tunnel = nil
        goCallback = nil
        teardownUDP()
    }

    private func dialThenApplySettings(
        profile: MasqueProfile,
        dialServer: String,
        remote: String,
        pipe: MobileDatagramPipe?,
        transport: String,
        onFailure: @escaping (String) -> Void
    ) {
        let cfg = MobileConfig()
        cfg.server = dialServer
        cfg.serverName = profile.serverName
        cfg.caPath = profile.caPath
        cfg.certPath = profile.certPath
        cfg.keyPath = profile.keyPath
        cfg.mtu = profile.mtu
        cfg.altPort = pipe == nil ? profile.altPort : 0
        cfg.bindInterface = ""
        cfg.transport = transport

        let cb = GoCallback(owner: self)
        goCallback = cb
        var dialErr: NSError?
        let t: MobileTunnel?
        if let pipe {
            t = MobileDialWithPipe(cfg, cb, pipe, &dialErr)
        } else {
            t = MobileDial(cfg, cb, &dialErr)
        }
        guard let t else {
            goCallback = nil
            let msg = dialErr?.localizedDescription ?? "dial failed"
            workQueue.async {
                onFailure(msg)
            }
            return
        }
        tunnel = t
        everDialed = true
        rememberAssigned(t)
        let used = Self.host(of: dialServer)
        if !used.isEmpty {
            lastServerIP = used
            lastRemote = used
        }

        let startBridge: () -> Void = { [weak self] in
            guard let self, !self.stopping else { return }
            if let pipe {
                self.rearmUDPRead(pipe: pipe)
            }
            do {
                try t.startPacketBridge()
            } catch {
                t.stop()
                self.tunnel = nil
                self.goCallback = nil
                if self.everDialed, self.startCompleted, !self.stopping {
                    self.reconnecting = false
                    self.scheduleReconnect(error.localizedDescription)
                    return
                }
                self.failAfterStart(error.localizedDescription, code: 11)
                return
            }
            self.reconnectAttempt = 0
            self.reconnecting = false
            // Cellular keeps flipping defaultPath for several seconds after
            // the first packet. A second reconnect here killed the new session
            // (VPS: CONNECT from the new IP, then client shutdown).
            self.ignorePathChangesUntil = Date().addingTimeInterval(8)
            self.lastPathKey = self.pathKey(self.defaultPath)
            self.pumpFromDevice()
            self.pumpToDevice()
            self.startPingTimer()
            AppGroup.defaults.set(true, forKey: AppGroup.defaultsSessionUp)
            AppGroup.defaults.synchronize()
            self.publishStatus("VPN active")
        }

        if routesApplied {
            workQueue.async(execute: startBridge)
            return
        }

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
                self.routesApplied = true
                startBridge()
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
        tearingDownUDP = true
        udpStateObs?.invalidate()
        udpStateObs = nil
        try? datagramPipe?.close()
        datagramPipe = nil
        udpWriter = nil
        udpSession?.cancel()
        udpSession = nil
        tearingDownUDP = false
    }

    /// QUIC going idle (server: "no recent network activity") used to cancel the
    /// whole VPN. Redial on a new UDP session and keep startTunnel completed.
    private func scheduleReconnect(_ reason: String) {
        workQueue.async { [weak self] in
            guard let self, !self.stopping, self.startCompleted, self.everDialed else { return }
            guard let profile = self.lastProfile, !self.lastServerIP.isEmpty else { return }
            guard !self.reconnecting else { return }
            self.reconnecting = true
            self.publishStatus("reconnecting: \(reason)")
            self.pingTimer?.cancel()
            self.pingTimer = nil
            self.tunnel?.stop()
            self.tunnel = nil
            self.goCallback = nil
            self.teardownUDP()
            MobileReleaseMemory()
            self.waitForPhysicalPath(tries: 0) { [weak self] in
                guard let self else { return }
                if self.stopping {
                    self.reconnecting = false
                    return
                }
                let host = Self.host(of: profile.server)
                var ips = Self.resolveServers(host)
                if ips.isEmpty, !self.lastServerIP.isEmpty {
                    ips = [self.lastServerIP]
                }
                self.connectCandidates(
                    profile: profile,
                    ips: ips,
                    port: self.lastPort,
                    portNum: self.lastPortNum
                ) { msg in
                    self.reconnecting = false
                    self.scheduleReconnectLater(msg)
                }
            }
        }
    }

    private func scheduleReconnectLater(_ reason: String) {
        guard !stopping, startCompleted, everDialed else { return }
        reconnectAttempt += 1
        let delay = min(8.0, Double(reconnectAttempt))
        publishStatus("reconnect in \(Int(delay))s: \(reason)")
        workQueue.asyncAfter(deadline: .now() + delay) { [weak self] in
            self?.scheduleReconnect(reason)
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        workQueue.async { [weak self] in
            guard let self else {
                completionHandler()
                return
            }
            self.stopping = true
            self.pathObs?.invalidate()
            self.pathObs = nil
            self.pingTimer?.cancel()
            self.pingTimer = nil
            self.tunnel?.stop()
            self.tunnel = nil
            self.goCallback = nil
            self.teardownUDP()
            AppGroup.defaults.set(0, forKey: AppGroup.defaultsPing)
            AppGroup.defaults.set("Disconnected", forKey: AppGroup.defaultsStatus)
            // A crash or a network drop leaves sessionUp set so the app can
            // block and reconnect. Disconnect, and turning the VPN off in
            // Settings, clears it so the block does not stick.
            if Self.userEnded(reason) {
                AppGroup.defaults.set(false, forKey: AppGroup.defaultsSessionUp)
            }
            AppGroup.defaults.synchronize()
            completionHandler()
        }
    }

    /// Stops the user asked for. Other reasons keep the kill-switch retry armed.
    private static func userEnded(_ reason: NEProviderStopReason) -> Bool {
        switch reason {
        case .userInitiated, .userLogout, .userSwitch, .providerDisabled,
             .configurationDisabled, .configurationRemoved, .authenticationCanceled, .superceded:
            return true
        default:
            return false
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
            guard let self else { return }
            if self.stopping {
                self.recordError(msg)
                return
            }
            self.scheduleReconnect(msg)
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
            let total = t.trafficBytes()
            if total > 0 {
                TrafficAccount.note(total: Int(total))
            }
            // Headroom before jetsam, plus Go heap: tells us whether a drop was
            // the process being killed or the network going away.
            let leftMB = Double(os_proc_available_memory()) / 1_048_576
            let heapMB = Double(MobileHeapKB()) / 1024
            self.publishStatus(String(
                format: "VPN active (free %.1f MB, heap %.1f MB)", leftMB, heapMB
            ))
        }
        timer.resume()
        pingTimer = timer
    }

    private func rememberAssigned(_ tunnel: MobileTunnel) {
        let v4 = tunnel.assignedAddr()
        if !v4.isEmpty { lastAssignedV4 = v4 }
        let v6 = tunnel.assignedAddrV6()
        if !v6.isEmpty { lastAssignedV6 = v6 }
    }

    private func networkSettings(for tunnel: MobileTunnel, profile: MasqueProfile, fallbackRemote: String) -> NEPacketTunnelNetworkSettings {
        var v4 = tunnel.assignedAddr()
        if v4.isEmpty { v4 = lastAssignedV4 }
        let got6 = tunnel.assignedAddrV6()
        let v6 = got6.isEmpty ? lastAssignedV6 : got6
        return routeSettings(v4: v4, v6: v6, remote: fallbackRemote, profile: profile)
    }

    private func routeSettings(v4: String, v6: String, remote: String, profile: MasqueProfile) -> NEPacketTunnelNetworkSettings {
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: remote)
        settings.mtu = NSNumber(value: profile.mtu)

        let ipv4 = NEIPv4Settings(addresses: [v4.isEmpty ? "10.8.0.254" : v4], subnetMasks: ["255.255.255.0"])
        ipv4.includedRoutes = [NEIPv4Route.default()]

        let ipv6: NEIPv6Settings
        if !v6.isEmpty {
            ipv6 = NEIPv6Settings(addresses: [v6], networkPrefixLengths: [64])
        } else {
            ipv6 = NEIPv6Settings(addresses: ["fd00::1"], networkPrefixLengths: [128])
        }
        // No IPv6 default: a second default route makes iOS rebuild the path
        // and the QUIC UDP session goes silent until idle-timeout.
        ipv6.includedRoutes = []
        Self.excludeServer(remote, ipv4: ipv4, ipv6: ipv6)
        settings.ipv4Settings = ipv4
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
                guard let pkt = tunnel.readPacket() else { return }
                if pkt.isEmpty { continue }
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

    /// Keep the server's own address off the tunnel. IPv6 has no default route;
    /// the exclude still covers a later settings update.
    private static func excludeServer(_ remote: String, ipv4: NEIPv4Settings, ipv6: NEIPv6Settings) {
        if remote.contains(":") {
            ipv6.excludedRoutes = [
                NEIPv6Route(destinationAddress: remote, networkPrefixLength: NSNumber(value: 128))
            ]
            return
        }
        if remote != "127.0.0.1" {
            ipv4.excludedRoutes = [NEIPv4Route(destinationAddress: remote, subnetMask: "255.255.255.255")]
        }
    }

    private static func dialServer(_ ip: String, _ port: String) -> String {
        if ip.contains(":") {
            return "[\(ip)]:\(port)"
        }
        return "\(ip):\(port)"
    }

    /// Put the address that last worked at the front. DNS order stays otherwise.
    private static func prefer(_ ip: String, in ips: [String]) -> [String] {
        guard !ip.isEmpty, let idx = ips.firstIndex(of: ip) else { return ips }
        var copy = ips
        copy.remove(at: idx)
        return [ip] + copy
    }

    /// A and AAAA. IPv6 is listed first, matching the shared client core.
    /// A literal address is returned as itself.
    private static func resolveServers(_ host: String) -> [String] {
        var v6buf = in6_addr()
        if host.withCString({ inet_pton(AF_INET6, $0, &v6buf) }) == 1 {
            return [host]
        }
        var v4buf = in_addr()
        if host.withCString({ inet_pton(AF_INET, $0, &v4buf) }) == 1 {
            return [host]
        }
        var hints = addrinfo()
        hints.ai_family = AF_UNSPEC
        hints.ai_socktype = SOCK_DGRAM
        var info: UnsafeMutablePointer<addrinfo>?
        guard getaddrinfo(host, nil, &hints, &info) == 0, let first = info else { return [] }
        defer { freeaddrinfo(info) }
        var v6: [String] = []
        var v4: [String] = []
        var ptr: UnsafeMutablePointer<addrinfo>? = first
        while let cur = ptr {
            var hostbuf = [CChar](repeating: 0, count: Int(NI_MAXHOST))
            if getnameinfo(
                cur.pointee.ai_addr,
                socklen_t(cur.pointee.ai_addrlen),
                &hostbuf,
                socklen_t(hostbuf.count),
                nil,
                0,
                NI_NUMERICHOST
            ) == 0 {
                let ip = String(cString: hostbuf).split(separator: "%", maxSplits: 1).first.map(String.init) ?? ""
                if cur.pointee.ai_family == AF_INET6, !ip.isEmpty, !ip.hasPrefix("fe80"), !v6.contains(ip) {
                    v6.append(ip)
                } else if cur.pointee.ai_family == AF_INET, !ip.isEmpty, !v4.contains(ip) {
                    v4.append(ip)
                }
            }
            ptr = cur.pointee.ai_next
        }
        return v6 + v4
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
    private var pending = 0
    private static let maxPending = 48

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
            // Unbounded writeDatagram during a speedtest balloons the
            // extension heap and iOS jetsams the process (~15 MB).
            if self.pending >= Self.maxPending { return }
            self.pending += 1
            self.session.writeDatagram(data) { [weak self] err in
                self?.queue.async {
                    guard let self else { return }
                    self.pending = max(0, self.pending - 1)
                    if let err {
                        self.owner?.noteUDPWriteError(err)
                    }
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
