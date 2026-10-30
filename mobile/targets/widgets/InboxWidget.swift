// CF-152: inbox widget (unread + assigned to me, latest conversations).

import SwiftUI
import WidgetKit

struct InboxWidget: Widget {
    static let kind = "ClickFixInbox"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: Self.kind, provider: ClickFixProvider(refreshesCounts: true)) { entry in
            InboxWidgetView(entry: entry)
        }
        .configurationDisplayName("Inbox")
        .description("Unread and assigned conversations at a glance.")
        .supportedFamilies([.systemSmall, .systemMedium, .systemLarge])
    }
}

struct InboxWidgetView: View {
    let entry: ClickFixEntry
    @Environment(\.widgetFamily) private var family

    var body: some View {
        Group {
            if let snapshot = entry.ready {
                if snapshot.can.inbox, let inbox = snapshot.inbox {
                    content(inbox)
                } else {
                    IssueView(issue: .noPermission)
                }
            } else {
                IssueView(issue: entry.issue ?? .unavailable)
            }
        }
        .clickFixWidget(entry)
    }

    @ViewBuilder
    private func content(_ inbox: InboxSummary) -> some View {
        switch family {
        case .systemSmall:
            small(inbox)
        case .systemLarge:
            large(inbox)
        default:
            medium(inbox)
        }
    }

    private func small(_ inbox: InboxSummary) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            WidgetHeader(title: "Inbox", symbol: "tray.fill")
            Spacer(minLength: 0)
            CountView(value: inbox.unread, label: "Unread", emphasized: true)
            Text("\(inbox.mine) assigned to me")
                .font(.caption2)
                .foregroundStyle(.secondary)
                .monospacedDigit()
                .lineLimit(1)
                .padding(.top, 2)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .widgetURL(DeepLink.inbox)
    }

    private func medium(_ inbox: InboxSummary) -> some View {
        HStack(alignment: .top, spacing: 12) {
            VStack(alignment: .leading, spacing: 6) {
                WidgetHeader(title: "Inbox", symbol: "tray.fill")
                Spacer(minLength: 0)
                CountView(value: inbox.unread, label: "Unread", emphasized: true)
                CountView(value: inbox.mine, label: "Mine")
            }
            .frame(width: 92, alignment: .leading)
            .frame(maxHeight: .infinity, alignment: .leading)

            rows(Array(inbox.top.prefix(2)), spacing: 10)
        }
        .widgetURL(DeepLink.inbox)
    }

    private func large(_ inbox: InboxSummary) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            WidgetHeader(title: "Inbox", symbol: "tray.fill", trailing: breachedLabel(inbox))
            HStack(alignment: .bottom, spacing: 24) {
                CountView(value: inbox.unread, label: "Unread", emphasized: true)
                CountView(value: inbox.mine, label: "Assigned to me")
                CountView(value: inbox.open, label: "Open")
                Spacer(minLength: 0)
            }
            Divider()
            rows(Array(inbox.top.prefix(5)), spacing: 8)
        }
        .widgetURL(DeepLink.inbox)
    }

    private func breachedLabel(_ inbox: InboxSummary) -> String? {
        inbox.breached > 0 ? String(localized: "\(inbox.breached) SLA breached") : nil
    }

    @ViewBuilder
    private func rows(_ items: [InboxItem], spacing: CGFloat) -> some View {
        if items.isEmpty {
            VStack(alignment: .leading, spacing: 4) {
                Image(systemName: "checkmark.circle")
                    .font(.title3)
                    .foregroundStyle(Brand.signalText)
                    .widgetAccentable()
                Text("You're all caught up")
                    .font(.subheadline.weight(.semibold))
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        } else {
            VStack(alignment: .leading, spacing: spacing) {
                ForEach(items) { item in
                    Link(destination: DeepLink.conversation(item.uuid)) {
                        ConversationRow(item: item, now: entry.date)
                    }
                }
                Spacer(minLength: 0)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
