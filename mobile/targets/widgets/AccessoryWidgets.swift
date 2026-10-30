// CF-152: lock screen accessories (circular / rectangular / inline).

import SwiftUI
import WidgetKit

struct UnreadAccessoryWidget: Widget {
    static let kind = "ClickFixUnreadAccessory"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: Self.kind, provider: ClickFixProvider(refreshesCounts: true)) { entry in
            UnreadAccessoryView(entry: entry)
        }
        .configurationDisplayName("Unread")
        .description("Unread and assigned conversations on the Lock Screen.")
        .supportedFamilies([.accessoryCircular, .accessoryRectangular, .accessoryInline])
    }
}

struct UnreadAccessoryView: View {
    let entry: ClickFixEntry
    @Environment(\.widgetFamily) private var family

    var body: some View {
        Group {
            if let snapshot = entry.ready {
                if snapshot.can.inbox, let inbox = snapshot.inbox {
                    content(inbox)
                } else {
                    AccessoryIssueView(issue: .noPermission)
                }
            } else {
                AccessoryIssueView(issue: entry.issue ?? .unavailable)
            }
        }
        .clickFixAccessory(entry)
    }

    @ViewBuilder
    private func content(_ inbox: InboxSummary) -> some View {
        switch family {
        case .accessoryCircular:
            ZStack {
                AccessoryWidgetBackground()
                VStack(spacing: 0) {
                    Image(systemName: "tray.fill")
                        .font(.caption)
                        .widgetAccentable()
                    Text(inbox.unread, format: .number)
                        .font(.system(.title3, design: .rounded, weight: .semibold))
                        .monospacedDigit()
                        .lineLimit(1)
                        .minimumScaleFactor(0.5)
                        .contentTransition(.numericText())
                }
                .padding(4)
            }
            .widgetLabel {
                Text("Unread")
            }
            .accessibilityLabel(Text("\(inbox.unread) unread"))
            .widgetURL(DeepLink.inbox)
        case .accessoryRectangular:
            VStack(alignment: .leading, spacing: 1) {
                Label {
                    Text(verbatim: "ClickFix")
                } icon: {
                    Image(systemName: "tray.fill")
                }
                .font(.headline)
                .widgetAccentable()
                Text("\(inbox.unread) unread")
                    .font(.body.weight(.semibold))
                    .monospacedDigit()
                    .contentTransition(.numericText())
                Text("\(inbox.mine) assigned to me")
                    .font(.caption)
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .widgetURL(DeepLink.inbox)
        default:
            Label {
                Text("\(inbox.unread) unread · \(inbox.mine) mine")
            } icon: {
                Image(systemName: "tray.fill")
            }
            .widgetURL(DeepLink.inbox)
        }
    }
}

struct NextAppointmentAccessoryWidget: Widget {
    static let kind = "ClickFixNextAppointment"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: Self.kind, provider: ClickFixProvider()) { entry in
            NextAppointmentAccessoryView(entry: entry)
        }
        .configurationDisplayName("Next appointment")
        .description("Your next appointment today on the Lock Screen.")
        .supportedFamilies([.accessoryCircular, .accessoryRectangular, .accessoryInline])
    }
}

struct NextAppointmentAccessoryView: View {
    let entry: ClickFixEntry
    @Environment(\.widgetFamily) private var family

    var body: some View {
        Group {
            if let snapshot = entry.ready {
                if snapshot.can.appointments {
                    content(snapshot.today.first { $0.isUpcoming(at: entry.date) })
                } else {
                    AccessoryIssueView(issue: .noPermission)
                }
            } else {
                AccessoryIssueView(issue: entry.issue ?? .unavailable)
            }
        }
        .clickFixAccessory(entry)
    }

    @ViewBuilder
    private func content(_ next: AppointmentItem?) -> some View {
        let url = next.map { DeepLink.appointment($0.uuid) } ?? DeepLink.url("/agenda")
        switch family {
        case .accessoryCircular:
            ZStack {
                AccessoryWidgetBackground()
                if let next {
                    VStack(spacing: 0) {
                        Image(systemName: next.kindSymbol)
                            .font(.caption)
                            .widgetAccentable()
                        Text(next.isInProgress(at: entry.date) ? String(localized: "Now") : WidgetFormat.time(next.startsAt))
                            .font(.system(.caption, design: .rounded, weight: .semibold))
                            .monospacedDigit()
                            .lineLimit(1)
                            .minimumScaleFactor(0.6)
                    }
                    .padding(4)
                } else {
                    Image(systemName: "calendar")
                        .font(.title3)
                }
            }
            .widgetURL(url)
        case .accessoryRectangular:
            VStack(alignment: .leading, spacing: 1) {
                if let next {
                    Label {
                        Text(next.isInProgress(at: entry.date) ? String(localized: "Now") : WidgetFormat.time(next.startsAt))
                            .monospacedDigit()
                    } icon: {
                        Image(systemName: next.kindSymbol)
                    }
                    .font(.headline)
                    .widgetAccentable()
                    Text(next.title.isEmpty ? String(localized: "Appointment") : next.title)
                        .font(.body.weight(.semibold))
                        .lineLimit(1)
                    if !next.with.isEmpty {
                        Text(next.with)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                } else {
                    Label("Today", systemImage: "calendar")
                        .font(.headline)
                        .widgetAccentable()
                    Text("No more appointments today")
                        .font(.caption)
                        .lineLimit(2)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .widgetURL(url)
        default:
            if let next {
                Label {
                    Text(verbatim: inlineText(next))
                } icon: {
                    Image(systemName: "calendar")
                }
                .widgetURL(url)
            } else {
                Label("No more appointments today", systemImage: "calendar")
                    .widgetURL(url)
            }
        }
    }

    private func inlineText(_ next: AppointmentItem) -> String {
        let title = next.title.isEmpty ? String(localized: "Appointment") : next.title
        if next.isInProgress(at: entry.date) {
            return String(localized: "Now") + " · " + title
        }
        let time = WidgetFormat.time(next.startsAt)
        return time.isEmpty ? title : time + " · " + title
    }
}
