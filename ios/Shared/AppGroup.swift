import Foundation

enum AppGroup {
    static let id = "group.com.next1971.masque"
    static let profileRelative = "certs"
    static let defaultsMTU = "mtu"
    static let defaultsTransport = "transport"
    static let defaultsKillSwitch = "killSwitch"
    /// True only after Connect with the kill switch on. Cleared on Disconnect.
    static let defaultsKillSwitchArmed = "killSwitchArmed"
    /// True after a session actually came up. A failed first connect stays false.
    static let defaultsSessionUp = "sessionUp"
    static let defaultsStatus = "status"
    static let defaultsPing = "pingMs"
    static let defaultsLastError = "lastError"
    /// Last VPN server IP that completed a handshake. Used when DNS is slow.
    static let defaultsLastServerIP = "lastServerIP"
    static let defaultsTrafficDay = "trafficDay"
    static let defaultsTrafficMonth = "trafficMonth"
    static let defaultsTrafficDayBytes = "trafficDayBytes"
    static let defaultsTrafficMonthBytes = "trafficMonthBytes"
    static let defaultsTrafficApplied = "trafficApplied"

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

/// Day and month tunnel totals in the App Group. The extension folds the pump
/// total in; the app only reads the label. A smaller total means the tunnel
/// process restarted and only the new bytes are added.
enum TrafficAccount {
    static func note(total: Int) {
        let d = AppGroup.defaults
        let day = dayKey()
        let month = monthKey()
        var dayBytes = d.integer(forKey: AppGroup.defaultsTrafficDayBytes)
        var monthBytes = d.integer(forKey: AppGroup.defaultsTrafficMonthBytes)
        let applied = d.integer(forKey: AppGroup.defaultsTrafficApplied)
        if d.string(forKey: AppGroup.defaultsTrafficDay) != day { dayBytes = 0 }
        if d.string(forKey: AppGroup.defaultsTrafficMonth) != month { monthBytes = 0 }
        var delta = total - applied
        if total < applied { delta = total }
        if delta < 0 { delta = 0 }
        dayBytes += delta
        monthBytes += delta
        d.set(day, forKey: AppGroup.defaultsTrafficDay)
        d.set(month, forKey: AppGroup.defaultsTrafficMonth)
        d.set(dayBytes, forKey: AppGroup.defaultsTrafficDayBytes)
        d.set(monthBytes, forKey: AppGroup.defaultsTrafficMonthBytes)
        d.set(total, forKey: AppGroup.defaultsTrafficApplied)
    }

    static func label() -> String {
        let d = AppGroup.defaults
        let dayBytes = d.string(forKey: AppGroup.defaultsTrafficDay) == dayKey()
            ? d.integer(forKey: AppGroup.defaultsTrafficDayBytes) : 0
        let monthBytes = d.string(forKey: AppGroup.defaultsTrafficMonth) == monthKey()
            ? d.integer(forKey: AppGroup.defaultsTrafficMonthBytes) : 0
        return "Today: \(format(dayBytes))\nMonth: \(format(monthBytes))"
    }

    private static func format(_ bytes: Int) -> String {
        String(format: "%.1f MB", Double(bytes) / 1_000_000)
    }

    private static func dayKey() -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = .current
        f.dateFormat = "yyyy-MM-dd"
        return f.string(from: Date())
    }

    private static func monthKey() -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = .current
        f.dateFormat = "yyyy-MM"
        return f.string(from: Date())
    }
}
