// CF-152: one timeline provider for every ClickFix widget. It reads the App
// Group snapshot; the inbox widgets may also refresh counts with the widget
// token (silent fallback to the snapshot).

import SwiftUI
import WidgetKit

enum WidgetIssue {
    case signedOut
    case noOrg
    case noPermission
    case stale
    case unavailable
}

struct ClickFixEntry: TimelineEntry {
    let date: Date
    let snapshot: WidgetSnapshot?
    let issue: WidgetIssue?
    let presence: PresenceStatus?
    let canChangePresence: Bool
    var isPlaceholder = false

    /// Ready data, or nil when the widget should show `issue`.
    var ready: WidgetSnapshot? { issue == nil ? snapshot : nil }
}

struct ClickFixProvider: TimelineProvider {
    /// Inbox / unread widgets refresh counts from the API.
    var refreshesCounts = false

    private static let refreshInterval: TimeInterval = 15 * 60

    func placeholder(in context: Context) -> ClickFixEntry {
        SampleData.entry(date: Date(), placeholder: true)
    }

    func getSnapshot(in context: Context, completion: @escaping (ClickFixEntry) -> Void) {
        let entry = Self.currentEntry(now: Date(), counts: nil)
        // The widget gallery shows sample data until the user has real data.
        if context.isPreview && entry.ready == nil {
            completion(SampleData.entry(date: Date(), placeholder: false))
        } else {
            completion(entry)
        }
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<ClickFixEntry>) -> Void) {
        let shouldFetch = refreshesCounts
        Task {
            let now = Date()
            var counts: WidgetCounts?
            if shouldFetch, let snapshot = SharedStore.readySnapshot(), snapshot.inbox != nil {
                counts = try? await ClickFixAPI.fetchCounts(expectedOrg: snapshot.orgSlug)
            }
            let first = Self.currentEntry(now: now, counts: counts)
            var entries = [first]
            for date in Self.transitionDates(after: now, snapshot: first.ready) {
                entries.append(Self.currentEntry(now: date, counts: counts, base: first))
            }
            let policy = TimelineReloadPolicy.after(now.addingTimeInterval(Self.refreshInterval))
            completion(Timeline(entries: entries, policy: policy))
        }
    }

    /// Entry for `now`. `base` reuses the already loaded data for future entries.
    static func currentEntry(now: Date, counts: WidgetCounts?, base: ClickFixEntry? = nil) -> ClickFixEntry {
        if let base {
            var issue = base.issue
            if issue == nil, counts == nil, base.snapshot?.isStale(now: now) == true {
                issue = .stale
            }
            return ClickFixEntry(
                date: now,
                snapshot: base.snapshot,
                issue: issue,
                presence: base.presence,
                canChangePresence: base.canChangePresence
            )
        }

        let canChange = WidgetKeychain.usableCredential() != nil
        switch SharedStore.loadSnapshot() {
        case .missing:
            return ClickFixEntry(date: now, snapshot: nil, issue: .signedOut, presence: nil, canChangePresence: false)
        case .invalid:
            return ClickFixEntry(date: now, snapshot: nil, issue: .unavailable, presence: nil, canChangePresence: false)
        case .loaded(var snapshot):
            if let counts, snapshot.inbox != nil {
                if let open = counts.open { snapshot.inbox?.open = max(0, open) }
                if let mine = counts.mine { snapshot.inbox?.mine = max(0, mine) }
                if let breached = counts.breached { snapshot.inbox?.breached = max(0, breached) }
            }
            let issue: WidgetIssue?
            switch snapshot.state {
            case .signedOut: issue = .signedOut
            case .noOrg: issue = .noOrg
            case .noPermission: issue = .noPermission
            case .ready:
                // Fresh counts from the API make old snapshot data acceptable.
                issue = snapshot.isStale(now: now) && counts == nil ? .stale : nil
            }
            return ClickFixEntry(
                date: now,
                snapshot: snapshot,
                issue: issue,
                presence: SharedStore.effectivePresence(for: snapshot),
                canChangePresence: canChange && snapshot.can.presence
            )
        }
    }

    /// Moments within the refresh window when a view must change on its own:
    /// an SLA turns orange (15 min left) or red (due), an appointment starts
    /// or ends.
    static func transitionDates(after now: Date, snapshot: WidgetSnapshot?) -> [Date] {
        guard let snapshot else { return [] }
        let horizon = now.addingTimeInterval(60 * 60)
        var dates: [Date] = []
        for item in snapshot.sla where !item.breached {
            guard let due = item.dueAt else { continue }
            dates.append(due.addingTimeInterval(-15 * 60))
            dates.append(due)
        }
        for appointment in snapshot.today {
            if let start = appointment.startsAt { dates.append(start) }
            if let end = appointment.endsAt { dates.append(end) }
        }
        let unique = Set(dates.filter { $0 > now && $0 <= horizon })
        return Array(unique.sorted().prefix(24))
    }
}

/// Gallery preview and placeholder content (redacted when placeholder).
enum SampleData {
    static func entry(date: Date, placeholder: Bool) -> ClickFixEntry {
        var entry = ClickFixEntry(
            date: date,
            snapshot: snapshot(now: date),
            issue: nil,
            presence: .online,
            canChangePresence: false
        )
        entry.isPlaceholder = placeholder
        return entry
    }

    static func snapshot(now: Date) -> WidgetSnapshot? {
        func iso(_ offset: TimeInterval) -> String { ClickFixDate.format(now.addingTimeInterval(offset)) }
        func uuid(_ n: Int) -> String { String(format: "00000000-0000-0000-0000-%012ld", n) }
        let top: [[String: Any]] = [
            ["uuid": uuid(1), "contact": "Mehmet Kaya", "channel": "whatsapp",
             "preview": String(localized: "When will my order arrive?"), "at": iso(-120), "unread": 2],
            ["uuid": uuid(2), "contact": "Elif Demir", "channel": "email",
             "preview": String(localized: "Could you resend the invoice?"), "at": iso(-1500), "unread": 1],
            ["uuid": uuid(3), "contact": "Can Öztürk", "channel": "webchat",
             "preview": String(localized: "Thanks, that fixed it."), "at": iso(-4000), "unread": 0],
        ]
        let inbox: [String: Any] = ["unread": 4, "mine": 7, "open": 23, "breached": 1, "top": top]
        let sla: [[String: Any]] = [
            ["uuid": uuid(4), "contact": "Zeynep Arslan", "channel": "email",
             "subject": String(localized: "Refund request"), "due_at": iso(-300), "breached": true],
            ["uuid": uuid(1), "contact": "Mehmet Kaya", "channel": "whatsapp",
             "subject": String(localized: "Order status"), "due_at": iso(600), "breached": false],
            ["uuid": uuid(5), "contact": "Burak Şahin", "channel": "instagram",
             "subject": String(localized: "Product question"), "due_at": iso(2400), "breached": false],
        ]
        let today: [[String: Any]] = [
            ["uuid": uuid(6), "title": String(localized: "Onboarding call"), "with": "Mehmet Kaya",
             "starts_at": iso(1800), "ends_at": iso(3600), "kind": "video",
             "join": "clickfix:///video/" + uuid(7)],
            ["uuid": uuid(8), "title": String(localized: "Product demo"), "with": "Elif Demir",
             "starts_at": iso(7200), "ends_at": iso(9000), "kind": "phone"],
        ]
        let org: [String: String] = ["slug": "sample", "name": "ClickFix"]
        let can: [String: Bool] = ["inbox": true, "appointments": true, "presence": true]
        let payload: [String: Any] = [
            "v": 1,
            "generated_at": iso(0),
            "state": "ready",
            "org": org,
            "can": can,
            "presence": "online",
            "inbox": inbox,
            "sla": sla,
            "today": today,
        ]
        guard let data = try? JSONSerialization.data(withJSONObject: payload) else { return nil }
        return try? JSONDecoder().decode(WidgetSnapshot.self, from: data)
    }
}
