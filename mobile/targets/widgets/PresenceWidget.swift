// CF-152: current status with iOS 17 interactive buttons. The buttons run
// SetPresenceIntent in the background with the widget token; WidgetKit
// reloads the timeline when the intent finishes.

import AppIntents
import SwiftUI
import WidgetKit

struct PresenceWidget: Widget {
    static let kind = "ClickFixPresence"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: Self.kind, provider: ClickFixProvider()) { entry in
            PresenceWidgetView(entry: entry)
        }
        .configurationDisplayName("Status")
        .description("See and change your availability.")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}

struct PresenceWidgetView: View {
    let entry: ClickFixEntry
    @Environment(\.widgetFamily) private var family

    private static let options: [PresenceStatusOption] = [.online, .away, .busy]

    var body: some View {
        Group {
            if let snapshot = entry.ready {
                if snapshot.can.presence {
                    content(snapshot)
                } else {
                    IssueView(issue: .noPermission)
                }
            } else {
                IssueView(issue: entry.issue ?? .unavailable)
            }
        }
        .clickFixWidget(entry)
    }

    private func content(_ snapshot: WidgetSnapshot) -> some View {
        let status = entry.presence ?? .offline
        let compact = family == .systemSmall
        return VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                Image(systemName: status.symbolName)
                    .foregroundStyle(status.tint)
                    .widgetAccentable()
                Text(status.localizedName)
                    .font(.headline)
                    .lineLimit(1)
                    .contentTransition(.interpolate)
                Spacer(minLength: 4)
                ClickFixBadge(size: 14)
            }
            .invalidatableContent()
            if let org = snapshot.orgName, !org.isEmpty {
                Text(org)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 0)
            if entry.canChangePresence {
                HStack(spacing: 6) {
                    ForEach(Self.options, id: \.self) { option in
                        PresenceButton(option: option, selected: option.status == status, compact: compact)
                    }
                }
            } else {
                Text("Open ClickFix to change your status")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .widgetURL(DeepLink.inbox)
    }
}

private struct PresenceButton: View {
    let option: PresenceStatusOption
    let selected: Bool
    let compact: Bool

    var body: some View {
        let status = option.status
        Button(intent: SetPresenceIntent(status: option)) {
            Group {
                if compact {
                    Image(systemName: status.symbolName)
                        .font(.body.weight(.semibold))
                } else {
                    Label(status.localizedName, systemImage: status.symbolName)
                        .font(.caption.weight(.semibold))
                        .lineLimit(1)
                        .minimumScaleFactor(0.8)
                }
            }
            .foregroundStyle(status.tint)
            .frame(maxWidth: .infinity, minHeight: 34)
            .background(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .fill(selected ? status.tint.opacity(0.22) : Color.primary.opacity(0.08))
            )
            .widgetAccentable(selected)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(status.localizedName))
    }
}
