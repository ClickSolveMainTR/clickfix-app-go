// CF-152: conversations close to (or past) their SLA, with live countdowns.
// The provider adds timeline entries at "15 min left" and "due" so the
// colours flip without waiting for the next refresh.

import SwiftUI
import WidgetKit

struct SLAWidget: Widget {
    static let kind = "ClickFixSLA"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: Self.kind, provider: ClickFixProvider()) { entry in
            SLAWidgetView(entry: entry)
        }
        .configurationDisplayName("SLA")
        .description("Conversations about to miss their response time.")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}

struct SLAWidgetView: View {
    let entry: ClickFixEntry
    @Environment(\.widgetFamily) private var family

    var body: some View {
        Group {
            if let snapshot = entry.ready {
                if snapshot.can.inbox {
                    content(snapshot.sla)
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
    private func content(_ items: [SLAItem]) -> some View {
        if items.isEmpty {
            VStack(alignment: .leading, spacing: 4) {
                WidgetHeader(title: "SLA", symbol: "timer")
                Spacer(minLength: 0)
                Image(systemName: "checkmark.seal")
                    .font(.title2)
                    .foregroundStyle(Brand.signalText)
                    .widgetAccentable()
                Text("No SLA at risk")
                    .font(.subheadline.weight(.semibold))
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
            .widgetURL(DeepLink.inbox)
        } else if family == .systemSmall, let first = items.first {
            small(first, more: items.count - 1)
        } else {
            medium(Array(items.prefix(3)))
        }
    }

    private func small(_ item: SLAItem, more: Int) -> some View {
        let tint = SLACountdown.tint(item, now: entry.date)
        return VStack(alignment: .leading, spacing: 2) {
            WidgetHeader(title: "SLA", symbol: "timer")
            Spacer(minLength: 0)
            Label {
                Text(item.isBreached(at: entry.date) ? LocalizedStringKey("Breached") : LocalizedStringKey("Due in"))
            } icon: {
                Image(systemName: SLACountdown.symbol(item, now: entry.date))
            }
            .font(.caption.weight(.semibold))
            .foregroundStyle(tint)
            SLACountdown(item: item, now: entry.date)
                .font(.system(.title, design: .rounded, weight: .semibold))
                .foregroundStyle(tint)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
                .widgetAccentable()
            Text(WidgetFormat.contact(item.contact))
                .font(.subheadline.weight(.semibold))
                .lineLimit(1)
            if more > 0 {
                Text("+\(more) more")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .widgetURL(DeepLink.conversation(item.uuid))
    }

    private func medium(_ items: [SLAItem]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            WidgetHeader(title: "SLA", symbol: "timer")
            ForEach(items) { item in
                Link(destination: DeepLink.conversation(item.uuid)) {
                    row(item)
                }
            }
            Spacer(minLength: 0)
        }
        .widgetURL(DeepLink.inbox)
    }

    private func row(_ item: SLAItem) -> some View {
        let tint = SLACountdown.tint(item, now: entry.date)
        return HStack(spacing: 8) {
            Image(systemName: SLACountdown.symbol(item, now: entry.date))
                .font(.subheadline)
                .foregroundStyle(tint)
                .frame(width: 18)
                .widgetAccentable()
            VStack(alignment: .leading, spacing: 0) {
                Text(WidgetFormat.contact(item.contact))
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                if !item.subject.isEmpty {
                    Text(item.subject)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .privacySensitive()
                }
            }
            Spacer(minLength: 4)
            SLACountdown(item: item, now: entry.date)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(tint)
                .multilineTextAlignment(.trailing)
                .frame(maxWidth: 72, alignment: .trailing)
        }
    }
}
