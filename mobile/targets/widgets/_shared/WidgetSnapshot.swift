// CF-152: snapshot v1 model (docs/mobile/widgets.md). Decoding is tolerant:
// unknown or malformed fields become nil / defaults and a bad list item is
// dropped instead of failing the whole snapshot.

import Foundation

enum PresenceStatus: String, Codable, CaseIterable {
    case online, away, busy, offline

    var symbolName: String {
        switch self {
        case .online: return "checkmark.circle.fill"
        case .away: return "moon.fill"
        case .busy: return "minus.circle.fill"
        case .offline: return "circle.dashed"
        }
    }

    var localizedName: String {
        switch self {
        case .online: return String(localized: "Online")
        case .away: return String(localized: "Away")
        case .busy: return String(localized: "Busy")
        case .offline: return String(localized: "Offline")
        }
    }
}

struct WidgetSnapshot: Decodable {
    enum State: String {
        case ready
        case signedOut = "signed_out"
        case noOrg = "no_org"
        case noPermission = "no_permission"
    }

    struct Capabilities {
        var inbox = false
        var appointments = false
        var presence = false
    }

    var version: Int
    var generatedAt: Date?
    var state: State
    var locale: String?
    var orgName: String?
    var orgSlug: String?
    var userName: String?
    var can: Capabilities
    var presence: PresenceStatus?
    var inbox: InboxSummary?
    var sla: [SLAItem]
    var today: [AppointmentItem]

    private enum CodingKeys: String, CodingKey {
        case version = "v"
        case generatedAt = "generated_at"
        case state, locale, org, user, can, presence, inbox, sla, today
    }

    private struct NameSlug: Decodable {
        let name: String?
        let slug: String?
    }

    private struct Can: Decodable {
        let inbox: Bool?
        let appointments: Bool?
        let presence: Bool?
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        version = c.lenient(Int.self, .version) ?? 0
        generatedAt = c.lenientDate(.generatedAt)
        // An unknown state is treated as signed out: the widget then asks the
        // user to open the app, which rewrites the snapshot.
        state = c.lenient(String.self, .state).flatMap(State.init(rawValue:)) ?? .signedOut
        locale = c.lenient(String.self, .locale)
        let org = c.lenient(NameSlug.self, .org)
        orgName = org?.name
        orgSlug = org?.slug
        userName = c.lenient(NameSlug.self, .user)?.name
        let can = c.lenient(Can.self, .can)
        self.can = Capabilities(
            inbox: can?.inbox ?? false,
            appointments: can?.appointments ?? false,
            presence: can?.presence ?? false
        )
        presence = c.lenient(PresenceStatus.self, .presence)
        inbox = c.lenient(InboxSummary.self, .inbox)
        sla = c.lenientArray(SLAItem.self, .sla)
        today = c.lenientArray(AppointmentItem.self, .today)
    }

    func isStale(now: Date, maxAge: TimeInterval = 6 * 60 * 60) -> Bool {
        guard let generatedAt else { return true }
        return now.timeIntervalSince(generatedAt) > maxAge
    }
}

struct InboxSummary: Decodable {
    var unread: Int
    var mine: Int
    var open: Int
    var breached: Int
    var top: [InboxItem]

    private enum CodingKeys: String, CodingKey { case unread, mine, open, breached, top }

    init(unread: Int, mine: Int, open: Int, breached: Int, top: [InboxItem]) {
        self.unread = unread
        self.mine = mine
        self.open = open
        self.breached = breached
        self.top = top
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        unread = max(0, c.lenient(Int.self, .unread) ?? 0)
        mine = max(0, c.lenient(Int.self, .mine) ?? 0)
        open = max(0, c.lenient(Int.self, .open) ?? 0)
        breached = max(0, c.lenient(Int.self, .breached) ?? 0)
        top = c.lenientArray(InboxItem.self, .top)
    }
}

struct InboxItem: Decodable, Identifiable {
    var uuid: String
    var contact: String
    var channel: String
    var preview: String
    var at: Date?
    var unread: Int
    var direction: String

    var id: String { uuid }

    private enum CodingKeys: String, CodingKey { case uuid, contact, channel, preview, at, unread, direction }

    init(uuid: String, contact: String, channel: String, preview: String, at: Date?, unread: Int, direction: String) {
        self.uuid = uuid
        self.contact = contact
        self.channel = channel
        self.preview = preview
        self.at = at
        self.unread = unread
        self.direction = direction
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        guard let uuid = c.lenient(String.self, .uuid).map(DeepLink.sanitizedId), !uuid.isEmpty else {
            throw DecodingError.dataCorruptedError(forKey: .uuid, in: c, debugDescription: "uuid")
        }
        self.uuid = uuid
        contact = c.lenient(String.self, .contact) ?? ""
        channel = c.lenient(String.self, .channel) ?? ""
        preview = c.lenient(String.self, .preview) ?? ""
        at = c.lenientDate(.at)
        unread = max(0, c.lenient(Int.self, .unread) ?? 0)
        direction = c.lenient(String.self, .direction) ?? "in"
    }
}

struct SLAItem: Decodable, Identifiable {
    var uuid: String
    var contact: String
    var channel: String
    var subject: String
    var dueAt: Date?
    var breached: Bool

    var id: String { uuid }

    private enum CodingKeys: String, CodingKey {
        case uuid, contact, channel, subject, breached
        case dueAt = "due_at"
    }

    init(uuid: String, contact: String, channel: String, subject: String, dueAt: Date?, breached: Bool) {
        self.uuid = uuid
        self.contact = contact
        self.channel = channel
        self.subject = subject
        self.dueAt = dueAt
        self.breached = breached
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        guard let uuid = c.lenient(String.self, .uuid).map(DeepLink.sanitizedId), !uuid.isEmpty else {
            throw DecodingError.dataCorruptedError(forKey: .uuid, in: c, debugDescription: "uuid")
        }
        self.uuid = uuid
        contact = c.lenient(String.self, .contact) ?? ""
        channel = c.lenient(String.self, .channel) ?? ""
        subject = c.lenient(String.self, .subject) ?? ""
        dueAt = c.lenientDate(.dueAt)
        breached = c.lenient(Bool.self, .breached) ?? false
    }

    /// Breached at `date`: the server flag, or the due time has passed.
    func isBreached(at date: Date) -> Bool {
        if breached { return true }
        guard let dueAt else { return false }
        return dueAt <= date
    }
}

struct AppointmentItem: Decodable, Identifiable {
    var uuid: String
    var title: String
    var with: String
    var startsAt: Date?
    var endsAt: Date?
    var kind: String
    var join: URL?

    var id: String { uuid }

    private enum CodingKeys: String, CodingKey {
        case uuid, title, with, kind, join
        case startsAt = "starts_at"
        case endsAt = "ends_at"
    }

    init(uuid: String, title: String, with: String, startsAt: Date?, endsAt: Date?, kind: String, join: URL?) {
        self.uuid = uuid
        self.title = title
        self.with = with
        self.startsAt = startsAt
        self.endsAt = endsAt
        self.kind = kind
        self.join = join
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        guard let uuid = c.lenient(String.self, .uuid).map(DeepLink.sanitizedId), !uuid.isEmpty else {
            throw DecodingError.dataCorruptedError(forKey: .uuid, in: c, debugDescription: "uuid")
        }
        self.uuid = uuid
        title = c.lenient(String.self, .title) ?? ""
        with = c.lenient(String.self, .with) ?? ""
        startsAt = c.lenientDate(.startsAt)
        endsAt = c.lenientDate(.endsAt)
        kind = c.lenient(String.self, .kind) ?? "custom_url"
        join = c.lenient(String.self, .join).flatMap(DeepLink.trustedURL)
    }

    /// Still relevant at `date` (not finished). Without an end time the
    /// appointment counts as an hour long.
    func isUpcoming(at date: Date) -> Bool {
        if let endsAt { return endsAt > date }
        if let startsAt { return startsAt.addingTimeInterval(60 * 60) > date }
        return true
    }

    func isInProgress(at date: Date) -> Bool {
        guard let startsAt else { return false }
        return startsAt <= date && isUpcoming(at: date)
    }

    var kindSymbol: String {
        switch kind {
        case "video": return "video.fill"
        case "phone": return "phone.fill"
        case "in_person": return "person.2.fill"
        default: return "link"
        }
    }
}

/// SF Symbol per channel type (backend channel `type` values).
enum ChannelSymbol {
    static func name(for channel: String) -> String {
        switch channel.lowercased() {
        case "whatsapp", "whatsapp_cloud", "whatsapp_business": return "message.fill"
        case "email", "mail", "imap", "gmail", "outlook": return "envelope.fill"
        case "webchat", "widget", "web", "livechat": return "bubble.left.and.bubble.right.fill"
        case "instagram": return "camera.fill"
        case "messenger", "facebook": return "message.circle.fill"
        case "telegram": return "paperplane.fill"
        case "sms": return "text.bubble.fill"
        case "voice", "phone", "call": return "phone.fill"
        case "slack", "teams", "google_chat": return "number"
        case "api": return "curlybraces"
        default: return "bubble.left.fill"
        }
    }
}

/// `clickfix://` deep links (docs/mobile/widgets.md). Routes are app paths
/// such as `/inbox`; the URL is `clickfix://` + route.
enum DeepLink {
    // A constant, valid literal: the force unwrap cannot fail at runtime.
    static let inbox = URL(string: "clickfix:///inbox")!

    static func url(_ route: String) -> URL {
        URL(string: "clickfix://" + route) ?? inbox
    }

    static func conversationRoute(_ uuid: String) -> String { "/conversation/" + sanitizedId(uuid) }
    static func appointmentRoute(_ uuid: String) -> String { "/appointment/" + sanitizedId(uuid) }

    static func searchRoute(_ query: String?) -> String {
        let trimmed = (query ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return "/search" }
        let allowed = CharacterSet.urlQueryAllowed.subtracting(CharacterSet(charactersIn: "&+=?#"))
        let encoded = trimmed.addingPercentEncoding(withAllowedCharacters: allowed) ?? ""
        return encoded.isEmpty ? "/search" : "/search?q=" + encoded
    }

    static func conversation(_ uuid: String) -> URL { url(conversationRoute(uuid)) }
    static func appointment(_ uuid: String) -> URL { url(appointmentRoute(uuid)) }

    /// Ids come from the snapshot: keep only uuid characters so a value can
    /// never change the route.
    static func sanitizedId(_ value: String) -> String {
        String(value.filter { $0.isHexDigit || $0 == "-" }.prefix(64))
    }

    /// Accept only app deep links and https links from external data.
    static func trustedURL(_ value: String) -> URL? {
        guard let url = URL(string: value), let scheme = url.scheme?.lowercased() else { return nil }
        return scheme == "clickfix" || scheme == "https" ? url : nil
    }

    /// `/search?q=x` from `clickfix:///search?q=x`.
    static func route(from url: URL) -> String? {
        guard url.scheme?.lowercased() == "clickfix" else { return nil }
        var route = url.path.isEmpty ? "/" : url.path
        if let query = url.query, !query.isEmpty { route += "?" + query }
        return route
    }
}

// MARK: - Tolerant decoding helpers

private struct Lossy<T: Decodable>: Decodable {
    let value: T?

    init(from decoder: Decoder) throws {
        value = try? T(from: decoder)
    }
}

extension KeyedDecodingContainer {
    func lenient<T: Decodable>(_ type: T.Type, _ key: Key) -> T? {
        guard let value = try? decodeIfPresent(type, forKey: key) else { return nil }
        return value
    }

    func lenientArray<T: Decodable>(_ type: T.Type, _ key: Key) -> [T] {
        guard let items = try? decodeIfPresent([Lossy<T>].self, forKey: key) else { return [] }
        return items.compactMap(\.value)
    }

    func lenientDate(_ key: Key) -> Date? {
        lenient(String.self, key).flatMap(ClickFixDate.parse)
    }
}
