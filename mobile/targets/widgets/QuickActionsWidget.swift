// CF-152: quick actions. Medium uses Links (one per tile). Small widgets have
// a single tap target for Links, so the small variant uses iOS 17 intent
// buttons; those intents open the app (openAppWhenRun, see AppRouter).

import AppIntents
import SwiftUI
import WidgetKit

struct QuickActionsWidget: Widget {
    static let kind = "ClickFixQuickActions"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: Self.kind, provider: ClickFixProvider()) { entry in
            QuickActionsWidgetView(entry: entry)
        }
        .configurationDisplayName("Quick actions")
        .description("Start a conversation, search or ask the assistant.")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}

struct QuickActionsWidgetView: View {
    let entry: ClickFixEntry
    @Environment(\.widgetFamily) private var family

    var body: some View {
        Group {
            // Signed out the actions would only land on the login screen.
            if entry.ready != nil || entry.issue == .stale {
                if family == .systemSmall {
                    small
                } else {
                    medium
                }
            } else {
                IssueView(issue: entry.issue ?? .unavailable)
            }
        }
        .clickFixWidget(entry)
    }

    private var small: some View {
        VStack(spacing: 6) {
            Button(intent: NewConversationIntent()) {
                QuickActionRow(title: "New", symbol: "square.and.pencil")
            }
            Button(intent: SearchIntent(query: nil)) {
                QuickActionRow(title: "Search", symbol: "magnifyingglass")
            }
            Button(intent: OpenAssistantIntent()) {
                QuickActionRow(title: "Assistant", symbol: "sparkles")
            }
        }
        .buttonStyle(.plain)
    }

    private var medium: some View {
        HStack(spacing: 8) {
            Link(destination: DeepLink.url("/new-conversation")) {
                QuickActionTile(title: "New conversation", symbol: "square.and.pencil")
            }
            Link(destination: DeepLink.url("/search")) {
                QuickActionTile(title: "Search", symbol: "magnifyingglass")
            }
            Link(destination: DeepLink.url("/assistant")) {
                QuickActionTile(title: "Assistant", symbol: "sparkles")
            }
        }
    }
}

private struct QuickActionRow: View {
    let title: LocalizedStringKey
    let symbol: String

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: symbol)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(Brand.signalText)
                .frame(width: 20)
                .widgetAccentable()
            Text(title)
                .font(.subheadline.weight(.semibold))
                .lineLimit(1)
                .minimumScaleFactor(0.8)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 10)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .fill(Color.primary.opacity(0.08))
        )
    }
}

private struct QuickActionTile: View {
    let title: LocalizedStringKey
    let symbol: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Image(systemName: symbol)
                .font(.title2.weight(.semibold))
                .foregroundStyle(Brand.signalText)
                .widgetAccentable()
            Spacer(minLength: 0)
            Text(title)
                .font(.subheadline.weight(.semibold))
                .lineLimit(2)
                .minimumScaleFactor(0.8)
        }
        .padding(10)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .fill(Color.primary.opacity(0.08))
        )
    }
}
