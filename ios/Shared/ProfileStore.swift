import Foundation

struct MasqueProfile {
    var server: String
    var serverName: String
    var dns: String
    var altPort: Int
    var caPath: String
    var certPath: String
    var keyPath: String
    var mtu: Int
}

enum ProfileStore {
    static func isConfigured() -> Bool {
        guard let dir = AppGroup.certsDir else { return false }
        return FileManager.default.fileExists(atPath: dir.appendingPathComponent("server.txt").path)
    }

    static func importText(_ content: String) throws {
        guard let dir = AppGroup.certsDir else {
            throw ProfileError.appGroupUnavailable
        }
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)

        let server = extractValue(content, key: "address") ?? extractValue(content, key: "server")
        guard let server, !server.isEmpty else { throw ProfileError.missing("server.address") }
        let name = extractValue(content, key: "name") ?? String(server.split(separator: ":").first ?? "")
        let dns = extractValue(content, key: "dns") ?? "1.1.1.1"
        let altPort = extractPort(content, key: "alt_port")
        guard let ca = extractBlock(content, key: "ca") else { throw ProfileError.missing("tls.ca") }
        guard let cert = extractBlock(content, key: "cert") else { throw ProfileError.missing("tls.cert") }
        guard let key = extractBlock(content, key: "key") else { throw ProfileError.missing("tls.key") }

        try (ca.trimmingCharacters(in: .whitespacesAndNewlines) + "\n").write(to: dir.appendingPathComponent("ca.crt"), atomically: true, encoding: .utf8)
        try (cert.trimmingCharacters(in: .whitespacesAndNewlines) + "\n").write(to: dir.appendingPathComponent("client.crt"), atomically: true, encoding: .utf8)
        try (key.trimmingCharacters(in: .whitespacesAndNewlines) + "\n").write(to: dir.appendingPathComponent("client.key"), atomically: true, encoding: .utf8)
        try "\(server)\n\(name)\n\(dns)\n\(altPort)\n".write(to: dir.appendingPathComponent("server.txt"), atomically: true, encoding: .utf8)
    }

    static func load() -> MasqueProfile? {
        guard let dir = AppGroup.certsDir else { return nil }
        let meta = dir.appendingPathComponent("server.txt")
        guard let text = try? String(contentsOf: meta, encoding: .utf8) else { return nil }
        let lines = text.split(whereSeparator: \.isNewline).map(String.init)
        guard lines.count >= 3 else { return nil }
        let mtu = AppGroup.defaults.object(forKey: AppGroup.defaultsMTU) as? Int ?? 1340
        var alt = 0
        if lines.count >= 4 {
            alt = Int(lines[3].trimmingCharacters(in: .whitespaces)) ?? 0
        }
        return MasqueProfile(
            server: lines[0].trimmingCharacters(in: .whitespaces),
            serverName: lines[1].trimmingCharacters(in: .whitespaces),
            dns: lines[2].trimmingCharacters(in: .whitespaces),
            altPort: alt,
            caPath: dir.appendingPathComponent("ca.crt").path,
            certPath: dir.appendingPathComponent("client.crt").path,
            keyPath: dir.appendingPathComponent("client.key").path,
            mtu: mtu
        )
    }

    static func saveMTU(_ mtu: Int) {
        AppGroup.defaults.set(mtu, forKey: AppGroup.defaultsMTU)
    }

    private static func extractPort(_ content: String, key: String) -> Int {
        if let quoted = extractValue(content, key: key), let n = Int(quoted) {
            return n
        }
        let escaped = NSRegularExpression.escapedPattern(for: key)
        let pattern = #"^\s*"# + escaped + #"\s*=\s*(\d+)"#
        guard let s = firstGroup(content, pattern: pattern, options: [.anchorsMatchLines]) else { return 0 }
        return Int(s) ?? 0
    }

    private static func extractValue(_ content: String, key: String) -> String? {
        let escaped = NSRegularExpression.escapedPattern(for: key)
        let pattern = #"^\s*"# + escaped + #"\s*=\s*"([^"]*)""#
        return firstGroup(content, pattern: pattern, options: [.anchorsMatchLines])
    }

    private static func extractBlock(_ content: String, key: String) -> String? {
        let escaped = NSRegularExpression.escapedPattern(for: key)
        let pattern = #"\b"# + escaped + #"\s*=\s*"""(.*?)""""#
        return firstGroup(content, pattern: pattern, options: [.dotMatchesLineSeparators])
    }

    private static func firstGroup(_ content: String, pattern: String, options: NSRegularExpression.Options) -> String? {
        guard let re = try? NSRegularExpression(pattern: pattern, options: options) else { return nil }
        let range = NSRange(content.startIndex..<content.endIndex, in: content)
        guard let match = re.firstMatch(in: content, options: [], range: range), match.numberOfRanges > 1,
              let r = Range(match.range(at: 1), in: content) else { return nil }
        return String(content[r])
    }
}

enum ProfileError: LocalizedError {
    case appGroupUnavailable
    case missing(String)

    var errorDescription: String? {
        switch self {
        case .appGroupUnavailable:
            return "App Group group.com.next1971.masque is not available. Enable it on the App ID in Apple Developer."
        case .missing(let key):
            return "missing \(key)"
        }
    }
}
