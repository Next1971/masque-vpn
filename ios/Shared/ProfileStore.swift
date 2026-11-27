import Foundation
import CryptoKit

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

/// One stored profile, for pickers in the host app.
struct ProfileInfo {
    var id: String
    var name: String
    var server: String
    var isActive: Bool
}

/// Several profiles can be stored at once, one directory each under
/// profiles\<id>\ in the App Group; the activeProfile default names the one
/// Connect uses. Transport, MTU, and kill switch stay per-device settings.
/// A pre-2.2 single-profile install (certs\ beside it) is copied into
/// profiles\ on first use; the legacy files stay in place so an older app
/// build keeps working after a rollback.
enum ProfileStore {
    static func isConfigured() -> Bool {
        activeID() != nil
    }

    /// Display name of the active profile, or "" when none is stored.
    static func activeName() -> String {
        guard let profile = load() else { return "" }
        return profile.serverName
    }

    // MARK: - Load

    static func load() -> MasqueProfile? {
        migrate()
        guard let id = activeID(), let dir = profileDir(id) else { return nil }
        return loadProfile(from: dir, mtu: currentMTU())
    }

    private static func loadProfile(from dir: URL, mtu: Int) -> MasqueProfile? {
        let meta = dir.appendingPathComponent("server.txt")
        guard let text = try? String(contentsOf: meta, encoding: .utf8) else { return nil }
        let lines = text.split(whereSeparator: \.isNewline).map(String.init)
        guard lines.count >= 3 else { return nil }
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

    // MARK: - Import

    /// Parses a self-contained profile and stores it under profiles\<id>\. An
    /// existing entry with the same server name and client certificate is
    /// updated in place; the imported profile becomes the active one.
    static func importText(_ content: String) throws {
        guard let root = AppGroup.profilesDir else {
            throw ProfileError.appGroupUnavailable
        }
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)

        let server = extractValue(content, key: "address") ?? extractValue(content, key: "server")
        guard let server, !server.isEmpty else { throw ProfileError.missing("server.address") }
        let name = extractValue(content, key: "name") ?? String(server.split(separator: ":").first ?? "")
        let dns = extractValue(content, key: "dns") ?? "1.1.1.1"
        let altPort = extractPort(content, key: "alt_port")
        guard let ca = extractBlock(content, key: "ca") else { throw ProfileError.missing("tls.ca") }
        guard let cert = extractBlock(content, key: "cert") else { throw ProfileError.missing("tls.cert") }
        guard let key = extractBlock(content, key: "key") else { throw ProfileError.missing("tls.key") }

        let id = profileID(name: name, certPEM: cert)
        let dir = root.appendingPathComponent(id, isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        try (ca.trimmingCharacters(in: .whitespacesAndNewlines) + "\n").write(to: dir.appendingPathComponent("ca.crt"), atomically: true, encoding: .utf8)
        try (cert.trimmingCharacters(in: .whitespacesAndNewlines) + "\n").write(to: dir.appendingPathComponent("client.crt"), atomically: true, encoding: .utf8)
        try (key.trimmingCharacters(in: .whitespacesAndNewlines) + "\n").write(to: dir.appendingPathComponent("client.key"), atomically: true, encoding: .utf8)
        try "\(server)\n\(name)\n\(dns)\n\(altPort)\n".write(to: dir.appendingPathComponent("server.txt"), atomically: true, encoding: .utf8)
        AppGroup.defaults.set(id, forKey: AppGroup.defaultsActiveProfile)
    }

    // MARK: - Picker support

    static func list() -> [ProfileInfo] {
        migrate()
        guard let root = AppGroup.profilesDir,
              let entries = try? FileManager.default.contentsOfDirectory(at: root, includingPropertiesForKeys: [.isDirectoryKey]) else {
            return []
        }
        let active = AppGroup.defaults.string(forKey: AppGroup.defaultsActiveProfile)
        var out: [ProfileInfo] = []
        for entry in entries where (try? entry.resourceValues(forKeys: [.isDirectoryKey]))?.isDirectory == true {
            guard FileManager.default.fileExists(atPath: entry.appendingPathComponent("server.txt").path) else { continue }
            var info = ProfileInfo(id: entry.lastPathComponent, name: "(invalid)", server: "", isActive: entry.lastPathComponent == active)
            if let lines = try? String(contentsOf: entry.appendingPathComponent("server.txt"), encoding: .utf8)
                .split(whereSeparator: \.isNewline).map(String.init), lines.count >= 2 {
                info.server = lines[0].trimmingCharacters(in: .whitespaces)
                let name = lines[1].trimmingCharacters(in: .whitespaces)
                if !name.isEmpty { info.name = name }
            }
            out.append(info)
        }
        return out.sorted { $0.name < $1.name }
    }

    /// Switches the active profile. Returns false when the id is not stored.
    @discardableResult
    static func setActive(_ id: String) -> Bool {
        migrate()
        guard let dir = profileDir(id),
              FileManager.default.fileExists(atPath: dir.appendingPathComponent("server.txt").path) else {
            return false
        }
        AppGroup.defaults.set(id, forKey: AppGroup.defaultsActiveProfile)
        return true
    }

    /// Removes one stored profile. Deleting the active one reactivates the
    /// first remaining, or clears the pointer when none are left.
    @discardableResult
    static func delete(_ id: String) -> Bool {
        migrate()
        guard let dir = profileDir(id),
              FileManager.default.fileExists(atPath: dir.appendingPathComponent("server.txt").path) else {
            return false
        }
        let wasActive = AppGroup.defaults.string(forKey: AppGroup.defaultsActiveProfile) == id
        do {
            try FileManager.default.removeItem(at: dir)
        } catch {
            return false
        }
        if wasActive {
            if let first = storedIDs().first {
                AppGroup.defaults.set(first, forKey: AppGroup.defaultsActiveProfile)
            } else {
                AppGroup.defaults.removeObject(forKey: AppGroup.defaultsActiveProfile)
            }
        }
        return true
    }

    // MARK: - Layout

    /// The id derives from the server name plus the client certificate hash, so
    /// re-importing the same bundle updates that entry instead of duplicating
    /// it, and two devices on one server stay separate profiles.
    static func profileID(name: String, certPEM: String) -> String {
        let digest = SHA256.hash(data: Data(certPEM.trimmingCharacters(in: .whitespacesAndNewlines).utf8))
        let hash = digest.map { String(format: "%02x", $0) }.joined().prefix(8)
        return slugify(name) + "-" + hash
    }

    private static func slugify(_ s: String) -> String {
        let lowered = s.trimmingCharacters(in: .whitespaces).lowercased()
        var out = ""
        for ch in lowered {
            if (ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9") {
                out.append(ch)
            } else {
                out.append("-")
            }
        }
        out = out.trimmingCharacters(in: CharacterSet(charactersIn: "-"))
        if out.count > 40 {
            out = String(out.prefix(40)).trimmingCharacters(in: CharacterSet(charactersIn: "-"))
        }
        return out.isEmpty ? "profile" : out
    }

    private static func storedIDs() -> [String] {
        guard let root = AppGroup.profilesDir,
              let entries = try? FileManager.default.contentsOfDirectory(at: root, includingPropertiesForKeys: [.isDirectoryKey]) else {
            return []
        }
        return entries
            .filter { (try? $0.resourceValues(forKeys: [.isDirectoryKey]))?.isDirectory == true }
            .filter { FileManager.default.fileExists(atPath: $0.appendingPathComponent("server.txt").path) }
            .map { $0.lastPathComponent }
            .sorted()
    }

    private static func profileDir(_ id: String) -> URL? {
        AppGroup.profilesDir?.appendingPathComponent(id, isDirectory: true)
    }

    /// A stale pointer (deleted profile) falls back to the first stored profile
    /// so the client keeps a usable selection.
    private static func activeID() -> String? {
        migrate()
        let ids = storedIDs()
        if ids.isEmpty { return nil }
        let saved = AppGroup.defaults.string(forKey: AppGroup.defaultsActiveProfile)
        if let saved, ids.contains(saved) { return saved }
        AppGroup.defaults.set(ids[0], forKey: AppGroup.defaultsActiveProfile)
        return ids[0]
    }

    /// Copies a pre-2.2 single-profile install into profiles\<id>\ and
    /// activates it, once. The legacy files are kept so an older app build
    /// still runs after a rollback.
    static func migrate() {
        let d = AppGroup.defaults
        if d.bool(forKey: AppGroup.defaultsMigratedProfiles) { return }
        guard let legacy = AppGroup.certsDir,
              FileManager.default.fileExists(atPath: legacy.appendingPathComponent("server.txt").path),
              let root = AppGroup.profilesDir else {
            d.set(true, forKey: AppGroup.defaultsMigratedProfiles)
            return
        }
        let meta = legacy.appendingPathComponent("server.txt")
        let lines = (try? String(contentsOf: meta, encoding: .utf8))?
            .split(whereSeparator: \.isNewline).map(String.init) ?? []
        guard lines.count >= 2 else {
            d.set(true, forKey: AppGroup.defaultsMigratedProfiles)
            return
        }
        let cert = (try? String(contentsOf: legacy.appendingPathComponent("client.crt"), encoding: .utf8)) ?? ""
        let id = profileID(name: lines[1].trimmingCharacters(in: .whitespaces), certPEM: cert)
        let dir = root.appendingPathComponent(id, isDirectory: true)
        do {
            try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            for name in ["ca.crt", "client.crt", "client.key", "server.txt"] {
                let src = legacy.appendingPathComponent(name)
                if FileManager.default.fileExists(atPath: src.path) {
                    try FileManager.default.copyItem(at: src, to: dir.appendingPathComponent(name))
                }
            }
            AppGroup.defaults.set(id, forKey: AppGroup.defaultsActiveProfile)
        } catch {
            // Leave the flag unset so the next launch retries.
            return
        }
        d.set(true, forKey: AppGroup.defaultsMigratedProfiles)
    }

    private static func currentMTU() -> Int {
        let stored = AppGroup.defaults.object(forKey: AppGroup.defaultsMTU) as? Int ?? 1340
        return min(1500, max(1280, stored))
    }

    static func saveMTU(_ mtu: Int) {
        AppGroup.defaults.set(mtu, forKey: AppGroup.defaultsMTU)
    }

    // MARK: - Parsing

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
