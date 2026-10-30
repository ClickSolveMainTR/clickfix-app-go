// CF-152: the two widget-token routes (`/v1/mobile/widget/*`). The widget
// token is narrow, short-lived and revocable; the user's refresh token never
// leaves the app's own Keychain group.

import Foundation
import WidgetKit

enum ClickFixAPIError: Error, CustomLocalizedStringResourceConvertible {
    case noCredential
    case expired
    case unauthorized
    case http(Int)
    case network

    var localizedStringResource: LocalizedStringResource {
        switch self {
        case .noCredential, .unauthorized:
            return "Open ClickFix and sign in to change your status."
        case .expired:
            return "Open ClickFix once to refresh your session."
        case .http(let code):
            return "ClickFix could not complete the request (\(code))."
        case .network:
            return "ClickFix could not be reached. Check your connection."
        }
    }
}

struct WidgetCounts {
    var open: Int?
    var mine: Int?
    var breached: Int?
}

enum ClickFixAPI {
    private static let session: URLSession = {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 10
        configuration.timeoutIntervalForResource = 15
        configuration.waitsForConnectivity = false
        configuration.httpCookieStorage = nil
        configuration.urlCache = nil
        return URLSession(configuration: configuration)
    }()

    /// `PUT /v1/mobile/widget/presence` with `{ "status": … }`.
    static func setPresence(_ status: PresenceStatus) async throws {
        let body = try JSONSerialization.data(withJSONObject: ["status": status.rawValue])
        _ = try await send(path: "/v1/mobile/widget/presence", method: "PUT", body: body, timeout: 10)
    }

    /// `GET /v1/mobile/widget/counts` (ConversationCounts in the envelope).
    /// Throws when the stored token belongs to another organization than the
    /// snapshot, so counts from two organizations are never mixed.
    static func fetchCounts(expectedOrg: String?) async throws -> WidgetCounts {
        guard let credential = WidgetKeychain.read() else { throw ClickFixAPIError.noCredential }
        if let expectedOrg, !expectedOrg.isEmpty, expectedOrg != credential.org {
            throw ClickFixAPIError.noCredential
        }
        let data = try await send(path: "/v1/mobile/widget/counts", method: "GET", body: nil, timeout: 5)
        guard let root = try JSONSerialization.jsonObject(with: data) as? [String: Any],
              let payload = root["data"] as? [String: Any]
        else { throw ClickFixAPIError.http(200) }
        return WidgetCounts(
            open: (payload["open"] as? NSNumber)?.intValue,
            mine: (payload["mine"] as? NSNumber)?.intValue,
            breached: (payload["breached"] as? NSNumber)?.intValue
        )
    }

    private static func send(path: String, method: String, body: Data?, timeout: TimeInterval) async throws -> Data {
        guard let credential = WidgetKeychain.read() else { throw ClickFixAPIError.noCredential }
        guard credential.isUsable else {
            throw credential.token.isEmpty ? ClickFixAPIError.noCredential : ClickFixAPIError.expired
        }
        var base = credential.apiURL.trimmingCharacters(in: .whitespacesAndNewlines)
        while base.hasSuffix("/") { base.removeLast() }
        guard let url = URL(string: base + path),
              let scheme = url.scheme?.lowercased(), scheme == "https" || scheme == "http"
        else { throw ClickFixAPIError.noCredential }

        var request = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: timeout)
        request.httpMethod = method
        request.setValue("Widget " + credential.token, forHTTPHeaderField: "Authorization")
        request.setValue(credential.org, forHTTPHeaderField: "X-Organization")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let body {
            request.httpBody = body
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }

        let result: (Data, URLResponse)
        do {
            result = try await session.data(for: request)
        } catch {
            throw ClickFixAPIError.network
        }
        let data = result.0
        guard let http = result.1 as? HTTPURLResponse else { throw ClickFixAPIError.network }
        switch http.statusCode {
        case 200..<300: return data
        case 401, 403: throw ClickFixAPIError.unauthorized
        default: throw ClickFixAPIError.http(http.statusCode)
        }
    }
}

/// Presence change shared by the widget button, the control and Siri.
enum PresenceService {
    static func apply(_ status: PresenceStatus) async throws {
        let previous = SharedStore.presenceOverride()
        // Optimistic: surfaces reloaded while the request runs already show
        // the new status; it is rolled back if the API refuses.
        SharedStore.writePresenceOverride(PresenceOverride(status: status, at: Date()))
        do {
            try await ClickFixAPI.setPresence(status)
        } catch {
            SharedStore.writePresenceOverride(previous)
            reloadSurfaces()
            throw error
        }
        reloadSurfaces()
    }

    static func reloadSurfaces() {
        WidgetCenter.shared.reloadAllTimelines()
        if #available(iOS 18.0, *) {
            ControlCenter.shared.reloadAllControls()
        }
    }
}
