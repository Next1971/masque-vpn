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
    /// packetFlow reads and writes. NEPacketTunnelFlow is not thread-safe.
    private let flowQueue = DispatchQueue(label: "com.next1971.masque.flow")
    private let flowEpochLock = NSLock()
    private var flowEpoch = 0
    /// Hands TCP bytes to Go off the Network Extension callback. deliver blocks
    /// when Go is behind; doing that on the connection callback also stalls writes.
    private let tcpDeliverQueue = DispatchQueue(label: "com.next1971.masque.tcp-deliver")
    private var startCompleted = false
    private var startHandler: ((Error?) -> Void)?

    private var udpSession: NWUDPSession?
    private var udpStateObs: NSKeyValueObservation?
    private var udpWriter: GoUDPWriter?
    private var datagramPipe: MobileDatagramPipe?
    private var tcpConnection: NWTCPConnection?
    private var tcpStateObs: NSKeyValueObservation?
    private var tcpWriter: GoTCPWriter?
    private var streamPipe: MobileStreamPipe?
    private var tcpReadPending = false
    private var tcpReadGeneration = 0
    private var udpWriteErrorLogged = false
    private var tcpWriteErrorLogged = false
    private var stopping = false
    private var reconnecting = false
    /// True only after the packet bridge is forwarding. Write errors during
    /// the handshake are dial failures, not a reason to tear the tunnel down.
    private var forwarding = false
    private var udpGeneration = 0
    private var tcpGeneration = 0
    private var udpReadID = 0
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
    private var tearingDownTCP = false
    private var reconnectAttempt = 0
    private var pathObs: NSKeyValueObservation?
    private var lastPathKey = ""
    private var lastPhysicalIP = ""
    private var ignorePathChangesUntil = Date.distantPast
    private var zeroRTTTicks = 0
    /// Last time the extension was actually running. A big gap means iOS froze
    /// the process in sleep and QUIC/TCP keepalives did not go out.
    private var lastAlive = Date()

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
        tcpWriteErrorLogged = false
        stopping = false
        reconnecting = false
        forwarding = false
        everDialed = false
        zeroRTTTicks = 0
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
        teardownTCP()
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

        udpWriteErrorLogged = false
        let writer = GoUDPWriter(session: session, queue: udpQueue, owner: self, generation: udpGeneration)
        udpWriter = writer
        var pipeErr: NSError?
        guard let pipe = MobileNewDatagramPipe(writer, serverIP, portNum, &pipeErr) else {
            onFailure(pipeErr?.localizedDescription ?? "UDP pipe failed")
            return
        }
        datagramPipe = pipe
        rearmUDPRead(pipe: pipe)

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
                // The route install right after dial also cancels this session;
                // reconnecting there RST the CONNECT that just succeeded.
                self?.reconnectUDPIfLive(msg)
            case .cancelled:
                finish(NSError(
                    domain: "masque",
                    code: 9,
                    userInfo: [NSLocalizedDescriptionKey: "UDP session cancelled"]
                ))
                self?.reconnectUDPIfLive("UDP session cancelled")
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

    /// A UDP failure while routes are being installed is the settings apply,
    /// not a dead path. Only a session that is already forwarding is replaced.
    private func reconnectUDPIfLive(_ reason: String) {
        workQueue.async { [weak self] in
            guard let self else { return }
            if self.tearingDownUDP || self.reconnecting || self.stopping { return }
            guard self.forwarding, self.tunnel != nil else { return }
            if Date() <= self.ignorePathChangesUntil { return }
            self.scheduleReconnect(reason)
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

    /// One address at a time, profile port then alt_port. The socket is
    /// NWTCPConnection so it stays on the physical path after the default
    /// route is installed. A BSD dial from Go is captured by the tunnel and
    /// the server then sees RST_STREAM CANCEL.
    private func tryTCP(
        profile: MasqueProfile,
        ips: [String],
        port: String,
        index: Int,
        lastError: String,
        onFailure: @escaping (String) -> Void
    ) {
        let portNum = Int(port) ?? 443
        let targets = udpTargets(ips: ips, port: port, portNum: portNum, altPort: profile.altPort)
        tryTCPTarget(
            profile: profile,
            targets: targets,
            index: index,
            lastError: lastError,
            onFailure: onFailure
        )
    }

    private func tryTCPTarget(
        profile: MasqueProfile,
        targets: [UDPTarget],
        index: Int,
        lastError: String,
        onFailure: @escaping (String) -> Void
    ) {
        if stopping {
            onFailure("stopped")
            return
        }
        if index >= targets.count {
            onFailure(lastError)
            return
        }
        let target = targets[index]
        ensureExcluded(target.ip, profile: profile) { [weak self] in
            guard let self else { return }
            self.publishStatus("opening TCP to \(target.ip):\(target.port)")
            self.openPhysicalTCP(host: target.ip, port: target.port) { [weak self] tcpErr in
                guard let self else { return }
                self.workQueue.async {
                    if let tcpErr {
                        self.tryTCPTarget(
                            profile: profile,
                            targets: targets,
                            index: index + 1,
                            lastError: tcpErr.localizedDescription,
                            onFailure: onFailure
                        )
                        return
                    }
                    guard let conn = self.tcpConnection else {
                        self.tryTCPTarget(
                            profile: profile,
                            targets: targets,
                            index: index + 1,
                            lastError: "TCP connection missing after ready",
                            onFailure: onFailure
                        )
                        return
                    }
                    self.tcpWriteErrorLogged = false
                    let writer = GoTCPWriter(connection: conn, owner: self, generation: self.tcpGeneration)
                    self.tcpWriter = writer
                    var pipeErr: NSError?
                    guard let stream = MobileNewStreamPipe(writer, target.ip, target.portNum, &pipeErr) else {
                        self.teardownTCP()
                        self.tryTCPTarget(
                            profile: profile,
                            targets: targets,
                            index: index + 1,
                            lastError: pipeErr?.localizedDescription ?? "TCP pipe failed",
                            onFailure: onFailure
                        )
                        return
                    }
                    self.streamPipe = stream
                    self.readTCP(stream: stream)
                    let dial = Self.dialServer(target.ip, target.port)
                    self.dialQueue.async {
                        self.dialThenApplySettings(
                            profile: profile,
                            dialServer: dial,
                            remote: target.ip,
                            pipe: nil,
                            stream: stream,
                            transport: "tcp",
                            onFailure: { msg in
                                self.abandonDialAttempt()
                                self.tryTCPTarget(
                                    profile: profile,
                                    targets: targets,
                                    index: index + 1,
                                    lastError: msg,
                                    onFailure: onFailure
                                )
                            }
                        )
                    }
                }
            }
        }
    }

    /// from: the Packet Tunnel API. enableTLS is false — Go does mTLS.
    private func openPhysicalTCP(host: String, port: String, completion: @escaping (Error?) -> Void) {
        teardownTCP()
        let endpoint = NWHostEndpoint(hostname: host, port: port)
        let conn = createTCPConnection(to: endpoint, enableTLS: false, tlsParameters: nil, delegate: nil)
        tcpConnection = conn

        var finished = false
        let finish: (Error?) -> Void = { [weak self] err in
            self?.workQueue.async {
                guard !finished else { return }
                finished = true
                completion(err)
            }
        }

        tcpStateObs = conn.observe(\.state, options: [.new]) { [weak self] c, _ in
            self?.publishStatus("TCP state \(c.state.rawValue)")
            switch c.state {
            case .connected:
                finish(nil)
            case .disconnected:
                let msg = c.error?.localizedDescription ?? "TCP connection disconnected"
                finish(NSError(domain: "masque", code: 9, userInfo: [NSLocalizedDescriptionKey: msg]))
                self?.reconnectTCPIfLive(msg, conn: c)
            case .cancelled:
                finish(NSError(
                    domain: "masque",
                    code: 9,
                    userInfo: [NSLocalizedDescriptionKey: "TCP connection cancelled"]
                ))
                self?.reconnectTCPIfLive("TCP connection cancelled", conn: c)
            default:
                break
            }
        }

        if conn.state == .connected {
            finish(nil)
        }

        workQueue.asyncAfter(deadline: .now() + 8) { [weak self] in
            guard let self, let conn = self.tcpConnection else { return }
            if conn.state != .connected {
                finish(NSError(
                    domain: "masque",
                    code: 9,
                    userInfo: [NSLocalizedDescriptionKey: "TCP connection not ready (state \(conn.state.rawValue))"]
                ))
            }
        }
    }

    private func reconnectTCPIfLive(_ reason: String, conn: NWTCPConnection? = nil) {
        workQueue.async { [weak self] in
            guard let self else { return }
            if let conn, self.tcpConnection !== conn { return }
            if self.tearingDownTCP || self.reconnecting || self.stopping { return }
            guard self.forwarding, self.tunnel != nil, self.everDialed, self.startCompleted else { return }
            if Date() <= self.ignorePathChangesUntil { return }
            self.scheduleReconnect(reason)
        }
    }

    private func readTCP(stream: MobileStreamPipe) {
        guard !tcpReadPending, !tearingDownTCP, !stopping, let conn = tcpConnection else { return }
        tcpReadPending = true
        let generation = tcpReadGeneration
        conn.readMinimumLength(1, maximumLength: 16 * 1024) { [weak self] data, err in
            guard let self else { return }
            // Return immediately. deliver blocks while Go is behind, and this
            // callback is also where write completions run. Blocking it stalls
            // the handshake and the app sits on "connecting" until the write
            // gives up (~8s) and the process is killed.
            let payload = (err == nil) ? data.flatMap { $0.isEmpty ? nil : Data($0) } : nil
            self.tcpDeliverQueue.async { [weak self] in
                if let payload {
                    stream.deliver(payload)
                }
                self?.workQueue.async {
                    guard let self else { return }
                    guard generation == self.tcpReadGeneration else { return }
                    self.tcpReadPending = false
                    if let err {
                        let text = err.localizedDescription.lowercased()
                        if text.contains("cancel") || text.contains("отмен") {
                            if self.tunnel != nil, !self.stopping, !self.tearingDownTCP {
                                self.readTCP(stream: stream)
                            }
                            return
                        }
                        if self.tunnel == nil {
                            self.recordError("TCP read: \(err.localizedDescription)")
                        } else {
                            self.reconnectTCPIfLive("TCP read: \(err.localizedDescription)")
                        }
                        return
                    }
                    self.readTCP(stream: stream)
                }
            }
        }
    }

    private func teardownTCP() {
        tearingDownTCP = true
        tcpGeneration &+= 1
        tcpReadGeneration += 1
        tcpReadPending = false
        tcpStateObs?.invalidate()
        tcpStateObs = nil
        try? streamPipe?.close()
        streamPipe = nil
        tcpWriter = nil
        tcpConnection?.cancel()
        tcpConnection = nil
        tearingDownTCP = false
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
        teardownTCP()
    }

    private func dialThenApplySettings(
        profile: MasqueProfile,
        dialServer: String,
        remote: String,
        pipe: MobileDatagramPipe?,
        stream: MobileStreamPipe? = nil,
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
        cfg.altPort = (pipe == nil && stream == nil) ? profile.altPort : 0
        cfg.bindInterface = ""
        cfg.transport = transport

        let cb = GoCallback(owner: self)
        goCallback = cb
        var dialErr: NSError?
        let t: MobileTunnel?
        if let pipe {
            t = MobileDialWithPipe(cfg, cb, pipe, &dialErr)
        } else if let stream {
            t = MobileDialWithStream(cfg, cb, stream, &dialErr)
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
            if let stream = self.streamPipe {
                self.readTCP(stream: stream)
            }
            do {
                try t.startPacketBridge()
            } catch {
                let dying = t
                let message = error.localizedDescription
                self.tunnel = nil
                self.goCallback = nil
                self.forwarding = false
                if self.everDialed, self.startCompleted, !self.stopping {
                    self.reconnecting = false
                    self.dialQueue.async { [weak self] in
                        dying.stop()
                        self?.workQueue.async {
                            self?.scheduleReconnect(message)
                        }
                    }
                    return
                }
                self.failAfterStart(message, code: 11)
                self.dialQueue.async { dying.stop() }
                return
            }
            self.reconnectAttempt = 0
            self.reconnecting = false
            self.forwarding = true
            self.udpWriteErrorLogged = false
            self.tcpWriteErrorLogged = false
            // Cellular keeps flipping defaultPath for several seconds after
            // the first packet. A second reconnect here killed the new session
            // (VPS: CONNECT from the new IP, then client shutdown).
            self.ignorePathChangesUntil = Date().addingTimeInterval(8)
            self.lastPathKey = self.pathKey(self.defaultPath)
            let epoch = self.flowEpochValue()
            self.pumpFromDevice(epoch: epoch)
            self.pumpToDevice(epoch: epoch)
            self.startPingTimer()
            AppGroup.defaults.removeObject(forKey: AppGroup.defaultsLastError)
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

    /// setReadHandler stops after a cancelled read (settings apply). Arm it
    /// again. Replacing the handler also cancels the read, so a handler must
    /// ignore the cancellation of the handler it just replaced.
    private func rearmUDPRead(pipe: MobileDatagramPipe) {
        guard let session = udpSession, !stopping, !tearingDownUDP else { return }
        udpReadID += 1
        let readID = udpReadID
        session.setReadHandler({ [weak self] datagrams, readErr in
            guard let self else { return }
            for d in datagrams ?? [] where !d.isEmpty {
                pipe.deliver(d)
            }
            guard let readErr else { return }
            let text = readErr.localizedDescription.lowercased()
            if text.contains("cancel") || text.contains("отмен") {
                self.workQueue.async {
                    guard readID == self.udpReadID else { return }
                    guard !self.stopping, !self.tearingDownUDP, !self.reconnecting else { return }
                    guard self.tunnel != nil, self.udpSession?.state == .ready else { return }
                    self.rearmUDPRead(pipe: pipe)
                }
                return
            }
            self.workQueue.async {
                guard readID == self.udpReadID else { return }
                if self.forwarding, self.tunnel != nil, !self.reconnecting, !self.stopping {
                    self.scheduleReconnect("UDP read: \(readErr.localizedDescription)")
                    return
                }
                guard self.tunnel == nil, !self.stopping else { return }
                self.recordError("UDP read: \(readErr.localizedDescription)")
            }
        }, maxDatagrams: 32)
    }

    private func teardownUDP() {
        tearingDownUDP = true
        udpGeneration &+= 1
        udpReadID += 1
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
    /// whole VPN. Redial on a new socket and keep startTunnel completed.
    /// The NE socket is cancelled before Go stop: Close writes a final packet,
    /// and doing that on a dead NWUDPSession/NWTCPConnection blocked this queue
    /// so the next Connect never returned.
    private func scheduleReconnect(_ reason: String) {
        workQueue.async { [weak self] in
            guard let self, !self.stopping, self.startCompleted, self.everDialed else { return }
            guard let profile = self.lastProfile, !self.lastServerIP.isEmpty else { return }
            guard !self.reconnecting else { return }
            self.reconnecting = true
            self.forwarding = false
            self.publishStatus("reconnecting: \(reason)")
            self.pingTimer?.cancel()
            self.pingTimer = nil
            _ = self.invalidateFlow()
            let dying = self.tunnel
            self.tunnel = nil
            self.goCallback = nil
            self.teardownUDP()
            self.teardownTCP()
            self.dialQueue.async { [weak self] in
                dying?.stop()
                MobileReleaseMemory()
                self?.workQueue.async {
                    guard let self else { return }
                    if self.stopping {
                        self.reconnecting = false
                        return
                    }
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

    /// iOS freezes this process and QUIC then dies after three quiet minutes.
    /// wake() is the only signal that the process is running again.
    override func wake() {
        workQueue.async { [weak self] in
            guard let self, !self.stopping, self.startCompleted, self.everDialed else { return }
            guard Date() > self.ignorePathChangesUntil else { return }
            let udpReady = self.udpSession?.state == .ready
            let tcpReady = self.tcpConnection?.state == .connected
            let rtt = self.tunnel?.rttMillis() ?? 0
            let frozen = Date().timeIntervalSince(self.lastAlive) > 20
            // A cached RTT stays non-zero across sleep, so it cannot prove the
            // session survived. Twenty seconds without a tick means the
            // process was frozen and the server will drop the idle QUIC conn.
            let pathReady = udpReady || tcpReady
            if self.tunnel == nil || rtt == 0 || !pathReady || frozen {
                self.zeroRTTTicks = 0
                self.scheduleReconnect(frozen ? "extension woke from sleep" : "extension woke")
            }
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        workQueue.async { [weak self] in
            guard let self else {
                completionHandler()
                return
            }
            self.stopping = true
            self.forwarding = false
            self.reconnecting = false
            self.pathObs?.invalidate()
            self.pathObs = nil
            self.pingTimer?.cancel()
            self.pingTimer = nil
            _ = self.invalidateFlow()
            let dying = self.tunnel
            self.tunnel = nil
            self.goCallback = nil
            self.teardownUDP()
            self.teardownTCP()
            AppGroup.defaults.set(0, forKey: AppGroup.defaultsPing)
            AppGroup.defaults.set("Disconnected", forKey: AppGroup.defaultsStatus)
            // A crash or a network drop leaves sessionUp set so the app can
            // block and reconnect. Disconnect, and turning the VPN off in
            // Settings, clears it so the block does not stick.
            if Self.userEnded(reason) {
                AppGroup.defaults.set(false, forKey: AppGroup.defaultsSessionUp)
            }
            AppGroup.defaults.synchronize()
            self.dialQueue.async {
                dying?.stop()
                completionHandler()
            }
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

    /// A dead NE socket reports ENOTCONN ("socket is not connected") or
    /// ECANCELED ("operation cancelled") on the write completion. State can
    /// still say ready/connected. Logging it and waiting for the 3-minute
    /// QUIC idle left the tunnel black-holed; the next Connect then blocked
    /// in Go Close on that socket.
    fileprivate func tcpWriteFailed(generation: Int, _ err: Error) {
        workQueue.async { [weak self] in
            guard let self, generation == self.tcpGeneration else { return }
            guard self.forwarding, !self.stopping, !self.tearingDownTCP, !self.reconnecting else { return }
            // The route install cancels in-flight writes. That is not a dead socket.
            if Self.isBenignCancel(err), Date() <= self.ignorePathChangesUntil { return }
            guard Self.isDeadSocket(err) || self.tcpConnection?.state != .connected else { return }
            if !self.tcpWriteErrorLogged {
                self.tcpWriteErrorLogged = true
                self.recordError("TCP write failed: \(err.localizedDescription)")
            }
            self.scheduleReconnect("TCP socket closed")
        }
    }

    fileprivate func udpWriteFailed(generation: Int, _ err: Error) {
        workQueue.async { [weak self] in
            guard let self, generation == self.udpGeneration else { return }
            guard self.forwarding, !self.stopping, !self.tearingDownUDP, !self.reconnecting else { return }
            // The route install cancels in-flight writes. That is not a dead socket.
            if Self.isBenignCancel(err), Date() <= self.ignorePathChangesUntil { return }
            guard Self.isDeadSocket(err) || self.udpSession?.state != .ready else { return }
            if !self.udpWriteErrorLogged {
                self.udpWriteErrorLogged = true
                self.recordError("UDP write failed: \(err.localizedDescription)")
            }
            self.scheduleReconnect("UDP socket closed")
        }
    }

    /// A read rearm or the route install cancels one write. ENOTCONN does not.
    private static func isBenignCancel(_ err: Error) -> Bool {
        let ns = err as NSError
        let text = ns.localizedDescription.lowercased()
        if text.contains("not connected") || text.contains("не подключен") || text.contains("socket") || text.contains("сокет") {
            return false
        }
        if ns.domain == NSPOSIXErrorDomain && ns.code == 89 { return true }
        return text.contains("cancel") || text.contains("отмен")
    }

    /// Localized text differs by language. POSIX codes do not.
    private static func isDeadSocket(_ err: Error) -> Bool {
        let ns = err as NSError
        if ns.domain == NSPOSIXErrorDomain {
            // EPIPE, ENETDOWN, ECONNRESET, ENOTCONN, ECANCELED.
            return ns.code == 32 || ns.code == 50 || ns.code == 54 || ns.code == 57 || ns.code == 89
        }
        let text = ns.localizedDescription.lowercased()
        return text.contains("not connected")
            || text.contains("не подключен")
            || text.contains("socket")
            || text.contains("сокет")
            || text.contains("reset")
            || text.contains("cancel")
            || text.contains("отмен")
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
            if msg == "forwarding started" || msg == "packet bridge ready" {
                self.publishStatus("VPN active")
                return
            }
            // Handshake lines are queued behind startBridge and used to
            // overwrite "VPN active" with "forwarding started".
            if self.everDialed,
               !msg.hasPrefix("reconnecting"),
               !msg.hasPrefix("reconnected"),
               !msg.hasPrefix("path changed"),
               !msg.hasPrefix("VPN active") {
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
        AppGroup.defaults.synchronize()
    }

    /// The main screen is a single line. Long stages wrap and shift the buttons.
    private func publishStatus(_ msg: String) {
        let shown = Self.shortStatus(msg)
        if shown == "connecting", tunnel != nil {
            return
        }
        AppGroup.defaults.set(shown, forKey: AppGroup.defaultsStatus)
        if let ms = tunnel?.rttMillis(), ms > 0 {
            AppGroup.defaults.set(Int(ms), forKey: AppGroup.defaultsPing)
        }
        AppGroup.defaults.synchronize()
    }

    private static func shortStatus(_ msg: String) -> String {
        if msg.hasPrefix("VPN active")
            || msg.hasPrefix("reconnected")
            || msg == "forwarding started"
            || msg == "packet bridge ready"
            || msg.hasPrefix("path changed") {
            return "VPN active"
        }
        if msg.hasPrefix("reconnect") {
            return "reconnecting"
        }
        if msg.hasPrefix("waiting")
            || msg.hasPrefix("physical")
            || msg.hasPrefix("no physical")
            || msg.hasPrefix("opening")
            || msg.hasPrefix("dialing")
            || msg.hasPrefix("UDP state")
            || msg.hasPrefix("TCP state")
            || msg.hasPrefix("starting") {
            return "connecting"
        }
        if msg.count > 22 {
            return "connecting"
        }
        return msg
    }

    private func startPingTimer() {
        // writeQueue is stuck inside readPacket for the whole session, so a
        // timer there never fires and traffic counters stay at zero.
        let timer = DispatchSource.makeTimerSource(queue: workQueue)
        timer.schedule(deadline: .now() + 2, repeating: 2)
        timer.setEventHandler { [weak self] in
            guard let self, let t = self.tunnel else { return }
            self.lastAlive = Date()
            let ms = t.rttMillis()
            if ms > 0 {
                AppGroup.defaults.set(Int(ms), forKey: AppGroup.defaultsPing)
            }
            self.workQueue.async {
                guard !self.stopping, self.everDialed, !self.reconnecting else { return }
                guard Date() > self.ignorePathChangesUntil else { return }
                if ms > 0 {
                    self.zeroRTTTicks = 0
                    return
                }
                self.zeroRTTTicks += 1
                // QUIC keepalive is 15s. A live session keeps a non-zero RTT.
                // Zero for half a minute means the socket died while we slept.
                if self.zeroRTTTicks >= 15 {
                    self.zeroRTTTicks = 0
                    self.scheduleReconnect("QUIC went silent")
                }
            }
            let total = t.trafficBytes()
            if total > 0 {
                TrafficAccount.note(total: Int(total))
            }
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

    /// Bumps the epoch so an in-flight read callback does not arm another read
    /// after reconnect, and the blocked ReadPacket loop exits.
    @discardableResult
    private func invalidateFlow() -> Int {
        var epoch = 0
        flowQueue.sync {
            self.flowEpochLock.lock()
            self.flowEpoch += 1
            epoch = self.flowEpoch
            self.flowEpochLock.unlock()
        }
        return epoch
    }

    private func flowEpochValue() -> Int {
        flowEpochLock.lock()
        let epoch = flowEpoch
        flowEpochLock.unlock()
        return epoch
    }

    private func pumpFromDevice(epoch: Int) {
        flowQueue.async { [weak self] in
            guard let self, epoch == self.flowEpochValue(), !self.stopping else { return }
            self.packetFlow.readPackets { [weak self] packets, _ in
                self?.flowQueue.async {
                    guard let self, epoch == self.flowEpochValue(), !self.stopping else { return }
                    guard let tunnel = self.tunnel else { return }
                    for pkt in packets {
                        _ = try? tunnel.writePacket(pkt)
                    }
                    self.pumpFromDevice(epoch: epoch)
                }
            }
        }
    }

    private func pumpToDevice(epoch: Int) {
        writeQueue.async { [weak self] in
            guard let self else { return }
            while epoch == self.flowEpochValue(), !self.stopping {
                guard let tunnel = self.tunnel else { return }
                guard let pkt = tunnel.readPacket() else { return }
                if pkt.isEmpty { continue }
                let proto = Self.ipProtocol(pkt)
                self.flowQueue.async { [weak self] in
                    guard let self, epoch == self.flowEpochValue(), !self.stopping else { return }
                    self.packetFlow.writePackets([pkt], withProtocols: [proto])
                }
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

/// Go calls writeStream on its HTTP/2 thread and will not read until the call
/// returns. Waiting here froze the dial for 8 seconds and then killed the app.
/// Bytes are copied onto a serial queue; that queue writes one chunk at a time
/// so the TCP stream stays in order without holding the Go thread.
private final class GoTCPWriter: NSObject, MobileStreamWriterProtocol {
    private let connection: NWTCPConnection
    private let queue = DispatchQueue(label: "com.next1971.masque.tcp-write")
    private weak var owner: PacketTunnelProvider?
    private let generation: Int
    private var pending: [Data] = []
    private var buffered = 0
    private var writing = false
    private static let maxBuffered = 1_048_576

    init(connection: NWTCPConnection, owner: PacketTunnelProvider, generation: Int) {
        self.connection = connection
        self.owner = owner
        self.generation = generation
        super.init()
    }

    func writeStream(_ p: Data?) throws {
        guard let data = p, !data.isEmpty else { return }
        if connection.state != .connected {
            let err = NSError(
                domain: NSPOSIXErrorDomain,
                code: 57,
                userInfo: [NSLocalizedDescriptionKey: "Socket is not connected"]
            )
            owner?.tcpWriteFailed(generation: generation, err)
            throw err
        }
        let copy = Data(data)
        var overflow = false
        queue.sync {
            if buffered + copy.count > Self.maxBuffered {
                overflow = true
                return
            }
            buffered += copy.count
            pending.append(copy)
            pumpLocked()
        }
        if overflow {
            let err = NSError(
                domain: "masque",
                code: 12,
                userInfo: [NSLocalizedDescriptionKey: "TCP write buffer full"]
            )
            owner?.tcpWriteFailed(generation: generation, err)
            throw err
        }
    }

    private func pumpLocked() {
        guard !writing, !pending.isEmpty else { return }
        writing = true
        let chunk = pending.removeFirst()
        connection.write(chunk) { [weak self] err in
            self?.queue.async {
                guard let self else { return }
                self.buffered = max(0, self.buffered - chunk.count)
                self.writing = false
                if let err {
                    self.owner?.tcpWriteFailed(generation: self.generation, err)
                }
                self.pumpLocked()
            }
        }
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
    private let generation: Int
    private let pendingLock = NSLock()
    private var pending = 0
    /// When the send window first filled. Cleared as soon as a completion lands.
    private var saturatedSince: Date?
    private static let maxPending = 48

    init(session: NWUDPSession, queue: DispatchQueue, owner: PacketTunnelProvider, generation: Int) {
        self.session = session
        self.queue = queue
        self.owner = owner
        self.generation = generation
        super.init()
    }

    func writeDatagram(_ p: Data?) throws {
        guard let data = p, !data.isEmpty else { return }
        if session.state != .ready {
            let err = NSError(
                domain: NSPOSIXErrorDomain,
                code: 57,
                userInfo: [NSLocalizedDescriptionKey: "Socket is not connected"]
            )
            owner?.udpWriteFailed(generation: generation, err)
            throw err
        }
        pendingLock.lock()
        if pending >= Self.maxPending {
            let now = Date()
            if saturatedSince == nil { saturatedSince = now }
            let stuck = now.timeIntervalSince(saturatedSince ?? now) > 2
            pendingLock.unlock()
            // A short burst is dropped: QUIC retransmits. Two seconds stuck at
            // the cap means completions stopped and keepalives will not leave.
            if !stuck { return }
            let err = NSError(
                domain: NSPOSIXErrorDomain,
                code: 57,
                userInfo: [NSLocalizedDescriptionKey: "Socket is not connected"]
            )
            owner?.udpWriteFailed(generation: generation, err)
            throw err
        }
        saturatedSince = nil
        pending += 1
        pendingLock.unlock()

        let gen = generation
        queue.async { [weak self] in
            guard let self else { return }
            self.session.writeDatagram(data) { [weak self] err in
                guard let self else { return }
                self.pendingLock.lock()
                self.pending = max(0, self.pending - 1)
                if self.pending < Self.maxPending { self.saturatedSince = nil }
                self.pendingLock.unlock()
                if let err {
                    self.owner?.udpWriteFailed(generation: gen, err)
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
