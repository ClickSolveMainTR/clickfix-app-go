// CF-152: Live Activity attributes. KEEP BYTE-FOR-BYTE IDENTICAL with
// mobile/targets/widgets/ClickFixActivityAttributes.swift
// (type names, property names, types and order). ActivityKit matches the
// app and the extension by type name and Codable shape, and the backend
// pushes `attributes-type: UrgentConversationAttributes` / content-state in
// exactly this shape. Dates are Double unix SECONDS (not Date: ActivityKit's
// JSON decoding of Date uses the 2001 reference date).

import ActivityKit
import Foundation

@available(iOS 16.2, *)
struct CallActivityAttributes: ActivityAttributes {
    public struct ContentState: Codable, Hashable {
        var muted: Bool
        var kind: String // "video" | "voice"
    }

    var room: String // video room uuid -> clickfix:///video/<room>
    var title: String
    var subtitle: String
    var startedAt: Double // unix seconds
}

@available(iOS 16.2, *)
struct UrgentConversationAttributes: ActivityAttributes {
    public struct ContentState: Codable, Hashable {
        var contact: String
        var channel: String
        var subject: String
        var preview: String
        var dueAt: Double // unix seconds, SLA target
        var breached: Bool
    }

    var conversation: String // uuid -> clickfix:///conversation/<uuid>
}
