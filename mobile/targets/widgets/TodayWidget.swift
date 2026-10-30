// CF-152: today's remaining appointments; the next one is prominent with a
// Join button for video appointments.

import SwiftUI
import WidgetKit

struct TodayWidget: Widget {
    static let kind = "ClickFixToday"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: Self.kind, provider: ClickFixProvider()) { entry in
            TodayWidgetView(entry: entry)
        }
        .configurationDisplayName("Today")
        .description("Your remaining appointments for today.")
        .supportedFamilies([.systemSmall, .systemMedium, .systemLarge])
    }
}

struct TodayWidgetView: View {
    let entry: ClickFixEntry
    @Environment(\.widgetFamily) private var family
    @Environment(\.widgetRenderingMode) private var renderingMode

    var body: some View {
        Group {
            if let snapshot = entry.ready {
                if snapshot.can.appointments {
                    content(snapshot.today.filter { $0.isUpcoming(at: entry.date) })
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
    private func content(_ items: [AppointmentItem]) -> some View {
        if let next = items.first {
            switch family {
            case .systemSmall:
                small(next, more: items.count - 1)
            case .systemLarge:
                VStack(alignment: .leading, spacing: 10) {
                    WidgetHeader(title: "Today", symbol: "calendar")
                    nextBlock(next)
                    if items.count > 1 {
                        Divider()
                        list(Array(items.dropFirst().prefix(5)))
                    }
                    Spacer(minLength: 0)
                }
                .widgetURL(DeepLink.url("/agenda"))
            default:
                HStack(alignment: .top, spacing: 12) {
                    VStack(alignment: .leading, spacing: 6) {
                        WidgetHeader(title: "Today", symbol: "calendar")
                        nextBlock(next)
                        Spacer(minLength: 0)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    if items.count > 1 {
                        list(Array(items.dropFirst().prefix(2)))
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                .widgetURL(DeepLink.url("/agenda"))
            }
        } else {
            VStack(alignment: .leading, spacing: 4) {
                WidgetHeader(title: "Today", symbol: "calendar")
                Spacer(minLength: 0)
                Image(systemName: "calendar")
                    .font(.title2)
                    .foregroundStyle(Brand.signalText)
                    .widgetAccentable()
                Text("No more appointments today")
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(2)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
            .widgetURL(DeepLink.url("/agenda"))
        }
    }

    private func small(_ next: AppointmentItem, more: Int) -> some View {
        // Small widgets have a single tap target: join a running video call,
        // otherwise open the appointment.
        let url = next.isInProgress(at: entry.date) ? (next.join ?? DeepLink.appointment(next.uuid)) : DeepLink.appointment(next.uuid)
        return VStack(alignment: .leading, spacing: 2) {
            WidgetHeader(title: "Today", symbol: "calendar")
            Spacer(minLength: 0)
            startLabel(next)
                .font(.system(.title2, design: .rounded, weight: .semibold))
            Text(title(next))
                .font(.subheadline.weight(.semibold))
                .lineLimit(2)
            if !next.with.isEmpty {
                Text(next.with)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            if more > 0 {
                Text("+\(more) more")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .widgetURL(url)
    }

    private func nextBlock(_ next: AppointmentItem) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            startLabel(next)
                .font(.system(.title2, design: .rounded, weight: .semibold))
            Link(destination: DeepLink.appointment(next.uuid)) {
                VStack(alignment: .leading, spacing: 1) {
                    Text(title(next))
                        .font(.subheadline.weight(.semibold))
                        .lineLimit(2)
                    if !next.with.isEmpty {
                        Label(next.with, systemImage: next.kindSymbol)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
            }
            if let join = next.join {
                Link(destination: join) {
                    joinLabel
                }
                .padding(.top, 2)
            }
        }
    }

    private var joinLabel: some View {
        Label("Join", systemImage: "video.fill")
            .font(.caption.weight(.semibold))
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .foregroundStyle(renderingMode == .fullColor ? Color.black : Color.primary)
            .background {
                // No full brand fill in tinted / StandBy modes.
                if renderingMode == .fullColor {
                    Capsule().fill(Brand.signal)
                } else {
                    Capsule().fill(Color.primary.opacity(0.15))
                }
            }
            .widgetAccentable()
    }

    private func list(_ items: [AppointmentItem]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(items) { item in
                Link(destination: DeepLink.appointment(item.uuid)) {
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        Text(WidgetFormat.time(item.startsAt))
                            .font(.caption.weight(.semibold))
                            .monospacedDigit()
                            .foregroundStyle(Brand.signalText)
                            .widgetAccentable()
                            .frame(minWidth: 40, alignment: .leading)
                        VStack(alignment: .leading, spacing: 0) {
                            Text(title(item))
                                .font(.caption.weight(.semibold))
                                .lineLimit(1)
                            if !item.with.isEmpty {
                                Text(item.with)
                                    .font(.caption2)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                            }
                        }
                    }
                }
            }
        }
    }

    @ViewBuilder
    private func startLabel(_ item: AppointmentItem) -> some View {
        if item.isInProgress(at: entry.date) {
            Text("Now")
                .foregroundStyle(Brand.signalText)
                .widgetAccentable()
        } else if let start = item.startsAt {
            Text(start, style: .time)
                .monospacedDigit()
        } else {
            Text("Today")
        }
    }

    private func title(_ item: AppointmentItem) -> String {
        item.title.isEmpty ? String(localized: "Appointment") : item.title
    }
}
