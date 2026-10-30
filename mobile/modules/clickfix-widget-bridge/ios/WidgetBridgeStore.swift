// CF-152: app side of the widget contract (docs/mobile/widgets.md). This pod
// cannot see the `_shared` sources of the widget target, so the few keys and
// id rules are repeated here; keep both in sync.

import Foundation
import Security
import WidgetKit

enum WidgetBridgeStore {
    static let snapshotKey = "clickfix.widget.snapshot.v1"
    static let presenceOverrideKey = "clickfix.widget.presenceOverride"
    static let pendingRouteKey = "clickfix.widget.pendingRoute"

    static let keychainService = "com.getclickfix.widget"
    static let keychainAccount = "credential"

    // MARK: Identifiers

    static var mainBundleId: String { Bundle.main.bundleIdentifier ?? "" }

    static var appGroup: String {
        infoString("ClickFixAppGroup") ?? "group." + mainBundleId
    }

    static var defaults: UserDefaults? {
        guard !mainBundleId.isEmpty else { return nil }
        return UserDefaults(suiteName: appGroup)
    }

    /// `$(AppIdentifierPrefix)<bundleId>.shared`, from Info.plist when the
    /// build expanded it, otherwise from the team prefix of our own default
    /// Keychain group.
    static var keychainGroup: String? {
        if let explicit = infoString("ClickFixKeychainGroup") { return explicit }
        guard let prefix = appIdentifierPrefix else { return nil }
        return prefix + mainBundleId + ".shared"
    }

    private static var cachedPrefix: String?

    static var appIdentifierPrefix: String? {
        if var prefix = infoString("ClickFixAppIdentifierPrefix") {
            if !prefix.hasSuffix(".") { prefix += "." }
            return prefix
        }
        if let cachedPrefix { return cachedPrefix }
        let discovered = discoverPrefix()
        cachedPrefix = discovered
        return discovered
    }

    private static func infoString(_ key: String) -> String? {
        guard let raw = Bundle.main.object(forInfoDictionaryKey: key) as? String else { return nil }
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if value.isEmpty || value.contains("$(") { return nil }
        return value
    }

    /// Adds a throwaway item without an access group: it lands in the app's
    /// default group (`TEAMID.<bundleId>`), whose first component is the
    /// team prefix. The item is deleted right away.
    private static func discoverPrefix() -> String? {
        let base: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: keychainService + ".probe",
            kSecAttrAccount as String: "team-prefix",
        ]
        defer { SecItemDelete(base as CFDictionary) }

        var add = base
        add[kSecValueData as String] = Data()
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        add[kSecReturnAttributes as String] = true
        var result: CFTypeRef?
        var status = SecItemAdd(add as CFDictionary, &result)
        if status == errSecDuplicateItem {
            var query = base
            query[kSecReturnAttributes as String] = true
            query[kSecMatchLimit as String] = kSecMatchLimitOne
            status = SecItemCopyMatching(query as CFDictionary, &result)
        }
        guard status == errSecSuccess,
              let attributes = result as? [String: Any],
              let group = attributes[kSecAttrAccessGroup as String] as? String,
              let dot = group.firstIndex(of: ".")
        else { return nil }
        return String(group[...dot])
    }

    // MARK: App Group

    static func writeSnapshot(_ json: String) {
        guard let defaults else { return }
        defaults.set(json, forKey: snapshotKey)
        // The snapshot is now the truth for presence (see contract).
        defaults.removeObject(forKey: presenceOverrideKey)
        reloadSurfaces()
    }

    static func readPresenceOverride() -> String? {
        defaults?.string(forKey: presenceOverrideKey)
    }

    static func consumePendingRoute() -> String? {
        guard let defaults, let route = defaults.string(forKey: pendingRouteKey) else { return nil }
        defaults.removeObject(forKey: pendingRouteKey)
        return route.isEmpty ? nil : route
    }

    static func setPendingRoute(_ route: String) {
        defaults?.set(route, forKey: pendingRouteKey)
    }

    static func reloadSurfaces() {
        WidgetCenter.shared.reloadAllTimelines()
        if #available(iOS 18.0, *) {
            ControlCenter.shared.reloadAllControls()
        }
    }

    // MARK: Shared Keychain

    /// Stores the widget token JSON in the shared group, or deletes it.
    /// Returns the OSStatus of the write (errSecSuccess on delete).
    @discardableResult
    static func setCredential(_ json: String?) -> OSStatus {
        // Delete in every group we can reach, so a stale copy cannot linger.
        let match: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: keychainService,
            kSecAttrAccount as String: keychainAccount,
        ]
        SecItemDelete(match as CFDictionary)

        guard let json, let data = json.data(using: .utf8) else {
            reloadSurfaces()
            return errSecSuccess
        }
        guard let group = keychainGroup else { return errSecMissingEntitlement }

        var add = match
        add[kSecAttrAccessGroup as String] = group
        add[kSecValueData as String] = data
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        let status = SecItemAdd(add as CFDictionary, nil)
        reloadSurfaces()
        return status
    }
}
