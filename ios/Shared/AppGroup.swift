import Foundation

enum AppGroup {
    static let id = "group.com.next1971.masque"
    static let profileRelative = "certs"
    static let defaultsMTU = "mtu"
    static let defaultsStatus = "status"
    static let defaultsPing = "pingMs"

    static var containerURL: URL? {
        FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: id)
    }

    static var defaults: UserDefaults {
        UserDefaults(suiteName: id) ?? .standard
    }

    static var certsDir: URL? {
        containerURL?.appendingPathComponent(profileRelative, isDirectory: true)
    }
}
