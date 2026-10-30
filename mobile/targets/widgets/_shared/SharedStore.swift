// CF-152: identifiers, App Group storage and the shared Keychain item.
// Compiled into the widget extension and the main app (`_shared`).
// Contract: docs/mobile/widgets.md.

import Foundation
import Security

/// Identifiers derived from the running bundle, so nothing hardcodes
/// `com.getclickfix.app` (white-label / staging bundle ids keep working).
enum ClickFixIds {
    /// Main app bundle id. Inside the extension the id is `<main>.widgets`.
    static var mainBundleId: String {
        let bundleId = Bundle.main.bundleIdentifier ?? ""
        guard Bundle.main.bundleURL.pathExtension == "appex",
              let dot = bundleId.lastIndex(of: ".")
        else { return bundleId }
        return String(bundleId[..<dot])
    }

    static var appGroup: String {
        if let explicit = infoString("ClickFixAppGroup") { return explicit }
        return "group." + mainBundleId
    }

    /// Team prefix such as `ABCDE12345.`; nil when the build left the
    /// `$(AppIdentifierPrefix)` variable unexpanded (unsigned builds).
    static var appIdentifierPrefix: String? {
        guard var prefix = infoString("ClickFixAppIdentifierPrefix") else { return nil }
        if !prefix.hasSuffix(".") { prefix += "." }
        return prefix
    }

    static var keychainGroup: String? {
        if let explicit = infoString("ClickFixKeychainGroup") { return explicit }
        guard let prefix = appIdentifierPrefix else { return nil }
        return prefix + mainBundleId + ".shared"
    }

    private static func infoString(_ key: String) -> String? {
        guard let raw = Bundle.main.object(forInfoDictionaryKey: key) as? String else { return nil }
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if value.isEmpty || value.contains("$(") { return nil }
        return value
    }
}

/// App Group `UserDefaults` keys written by the app and the intents.
enum SharedStore {
    static let snapshotKey = "clickfix.widget.snapshot.v1"
    static let presenceOverrideKey = "clickfix.widget.presenceOverride"
    static let pendingRouteKey = "clickfix.widget.pendingRoute"

    static var defaults: UserDefaults? { UserDefaults(suiteName: ClickFixIds.appGroup) }

    enum SnapshotLoad {
        case missing
        case invalid
        case loaded(WidgetSnapshot)
    }

    static func loadSnapshot() -> SnapshotLoad {
        guard let raw = defaults?.string(forKey: snapshotKey), !raw.isEmpty else { return .missing }
        guard let data = raw.data(using: .utf8),
              let snapshot = try? JSONDecoder().decode(WidgetSnapshot.self, from: data),
              snapshot.version == 1
        else { return .invalid }
        return .loaded(snapshot)
    }

    /// Convenience for intents and entity queries that only need ready data.
    static func readySnapshot() -> WidgetSnapshot? {
        if case .loaded(let snapshot) = loadSnapshot(), snapshot.state == .ready {
            return snapshot
        }
        return nil
    }

    static func presenceOverride() -> PresenceOverride? {
        guard let raw = defaults?.string(forKey: presenceOverrideKey),
              let data = raw.data(using: .utf8)
        else { return nil }
        return try? JSONDecoder().decode(PresenceOverride.self, from: data)
    }

    static func writePresenceOverride(_ value: PresenceOverride?) {
        guard let defaults else { return }
        guard let value else {
            defaults.removeObject(forKey: presenceOverrideKey)
            return
        }
        let payload: [String: String] = ["status": value.status.rawValue, "at": ClickFixDate.format(value.at)]
        if let data = try? JSONSerialization.data(withJSONObject: payload),
           let json = String(data: data, encoding: .utf8) {
            defaults.set(json, forKey: presenceOverrideKey)
        }
    }

    /// Route such as `/search?q=x`; the app reads and clears it on foreground.
    static func setPendingRoute(_ route: String) {
        defaults?.set(route, forKey: pendingRouteKey)
    }

    /// Presence to show: the override wins when it is newer than the snapshot
    /// (an intent changed it after the app last wrote the snapshot).
    static func effectivePresence(for snapshot: WidgetSnapshot?) -> PresenceStatus? {
        let fromSnapshot = snapshot?.presence
        guard let override = presenceOverride() else { return fromSnapshot }
        guard let generatedAt = snapshot?.generatedAt else { return override.status }
        return override.at > generatedAt ? override.status : fromSnapshot
    }
}

struct PresenceOverride: Decodable {
    let status: PresenceStatus
    let at: Date

    init(status: PresenceStatus, at: Date) {
        self.status = status
        self.at = at
    }

    private enum CodingKeys: String, CodingKey { case status, at }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        guard let status = c.lenient(PresenceStatus.self, .status) else {
            throw DecodingError.dataCorruptedError(forKey: .status, in: c, debugDescription: "status")
        }
        self.status = status
        self.at = c.lenientDate(.at) ?? .distantPast
    }
}

/// Widget token written by the app (`setCredential`). Read only here.
struct WidgetCredential: Decodable {
    let token: String
    let expiresAt: Date?
    let apiURL: String
    let org: String

    var isUsable: Bool {
        guard !token.isEmpty, !apiURL.isEmpty, !org.isEmpty else { return false }
        if let expiresAt, expiresAt <= Date() { return false }
        return true
    }

    private enum CodingKeys: String, CodingKey {
        case token, org
        case expiresAt = "expires_at"
        case apiURL = "api_url"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        token = c.lenient(String.self, .token) ?? ""
        apiURL = c.lenient(String.self, .apiURL) ?? ""
        org = c.lenient(String.self, .org) ?? ""
        expiresAt = c.lenientDate(.expiresAt)
    }
}

enum WidgetKeychain {
    static let service = "com.getclickfix.widget"
    static let account = "credential"

    /// No access group in the query: SecItemCopyMatching then searches every
    /// group this process is entitled to, so a missing or unexpanded team
    /// prefix in Info.plist does not break reads.
    static func read() -> WidgetCredential? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess, let data = result as? Data else { return nil }
        return try? JSONDecoder().decode(WidgetCredential.self, from: data)
    }

    static func usableCredential() -> WidgetCredential? {
        guard let credential = read(), credential.isUsable else { return nil }
        return credential
    }
}

enum ClickFixDate {
    private static let fractional: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let plain: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()

    static func parse(_ value: String) -> Date? {
        fractional.date(from: value) ?? plain.date(from: value)
    }

    static func format(_ date: Date) -> String {
        plain.string(from: date)
    }
}
