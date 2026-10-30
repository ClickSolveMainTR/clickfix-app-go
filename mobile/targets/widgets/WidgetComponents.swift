// CF-152: building blocks shared by the ClickFix widgets. System fonts and
// SF Symbols, plus the ClickFix cursor mark as the brand signature.

import SwiftUI
import WidgetKit

/// Colour sets generated from expo-target.config.js. Signal green is a fill
/// colour; green text uses `signalText` (contrast on light backgrounds).
enum Brand {
    static let signal = Color("brandSignal")
    static let signalText = Color("brandSignalText")
    static let ink = Color("brandInk")
    static let background = Color("widgetBackground")
}

/// The ClickFix cursor from the brand kit (`clickfix-mark-*.svg`,
/// viewBox 1280x744), drawn as a shape so it stays sharp at every size.
struct ClickFixCursor: Shape {
    func path(in rect: CGRect) -> Path {
        let sx = rect.width / 1280, sy = rect.height / 744
        func p(_ x: CGFloat, _ y: CGFloat) -> CGPoint {
            CGPoint(x: rect.minX + x * sx, y: rect.minY + y * sy)
        }
        var path = Path()
        path.move(to: p(64, 744))
        path.addLine(to: p(0, 0))
        path.addLine(to: p(1280, 168))
        path.addLine(to: p(288, 176))
        path.closeSubpath()
        return path
    }
}

/// App icon in miniature: black cursor on the signal green tile
/// (`clickfix-tile-green.svg`). In tinted / StandBy modes it follows the
/// system accent like the other accentable parts.
struct ClickFixBadge: View {
    var size: CGFloat = 16
    @Environment(\.widgetRenderingMode) private var renderingMode

    /// Kit green `#03FF13`; a fill, never text.
    private static let kitGreen = Color(red: 3 / 255, green: 1, blue: 19 / 255)

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: size * 0.24, style: .continuous)
                .fill(renderingMode == .fullColor ? Self.kitGreen : Color.primary)
                .widgetAccentable()
            ClickFixCursor()
                .fill(Color.black)
                .frame(width: size * 0.62, height: size * 0.62 * 744 / 1280)
                .offset(x: size * 0.016)
                // Tinted modes: punch the cursor out of the accent tile.
                .blendMode(renderingMode == .fullColor ? .normal : .destinationOut)
        }
        .compositingGroup()
        .frame(width: size, height: size)
        .accessibilityLabel(Text(verbatim: "ClickFix"))
    }
}

extension PresenceStatus {
    var tint: Color {
        switch self {
        case .online: return .green
        case .away: return .orange
        case .busy: return .red
        case .offline: return .gray
        }
    }
}

extension View {
    /// Standard widget chrome: brand background (removed by the system in
    /// StandBy / tinted modes) and placeholder redaction.
    func clickFixWidget(_ entry: ClickFixEntry) -> some View {
        redacted(reason: entry.isPlaceholder ? .placeholder : [])
            .containerBackground(for: .widget) { Brand.background }
    }

    /// Lock screen accessories draw their own background.
    func clickFixAccessory(_ entry: ClickFixEntry) -> some View {
        redacted(reason: entry.isPlaceholder ? .placeholder : [])
            .containerBackground(for: .widget) { Color.clear }
    }
}

enum WidgetFormat {
    /// `14:05` today, `29 Oct` otherwise.
    static func shortTime(_ date: Date, now: Date) -> String {
        if Calendar.current.isDate(date, inSameDayAs: now) {
            return date.formatted(date: .omitted, time: .shortened)
        }
        return date.formatted(.dateTime.day().month(.abbreviated))
    }

    static func time(_ date: Date?) -> String {
        guard let date else { return "" }
        return date.formatted(date: .omitted, time: .shortened)
    }

    static func contact(_ name: String) -> String {
        name.isEmpty ? String(localized: "Unknown contact") : name
    }
}

/// Header line: symbol + title, the way Apple's widgets label themselves.
struct WidgetHeader: View {
    let title: LocalizedStringKey
    let symbol: String
    var trailing: String? = nil

    var body: some View {
        HStack(spacing: 4) {
            Image(systemName: symbol)
                .foregroundStyle(Brand.signalText)
                .widgetAccentable()
            Text(title)
                .lineLimit(1)
            Spacer(minLength: 4)
            if let trailing {
                Text(trailing)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            ClickFixBadge(size: 14)
        }
        .font(.caption.weight(.semibold))
    }
}

/// Logged out / no organization / no permission / stale: a deliberate,
/// calm state with one line and a way into the app.
struct IssueView: View {
    let issue: WidgetIssue
    @Environment(\.widgetFamily) private var family

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .top) {
                Image(systemName: Self.symbol(issue))
                    .font(.title2)
                    .foregroundStyle(Brand.signalText)
                    .widgetAccentable()
                Spacer(minLength: 4)
                ClickFixBadge(size: 18)
            }
            Spacer(minLength: 0)
            Text(Self.message(issue))
                .font(.subheadline.weight(.semibold))
                .lineLimit(family == .systemSmall ? 3 : 2)
                .fixedSize(horizontal: false, vertical: true)
            Text("Open ClickFix")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .widgetURL(DeepLink.inbox)
    }

    static func symbol(_ issue: WidgetIssue) -> String {
        switch issue {
        case .signedOut: return "person.crop.circle"
        case .noOrg: return "building.2"
        case .noPermission: return "lock.fill"
        case .stale: return "arrow.clockwise"
        case .unavailable: return "arrow.triangle.2.circlepath"
        }
    }

    static func message(_ issue: WidgetIssue) -> LocalizedStringKey {
        switch issue {
        case .signedOut: return "Sign in to ClickFix to see your work here."
        case .noOrg: return "Choose an organization in ClickFix."
        case .noPermission: return "Your role cannot view this."
        case .stale: return "Open ClickFix to refresh."
        case .unavailable: return "Update ClickFix to use this widget."
        }
    }
}

/// Compact issue state for lock screen accessories.
struct AccessoryIssueView: View {
    let issue: WidgetIssue
    @Environment(\.widgetFamily) private var family

    var body: some View {
        switch family {
        case .accessoryCircular:
            ZStack {
                AccessoryWidgetBackground()
                Image(systemName: IssueView.symbol(issue))
                    .font(.title3)
            }
            .widgetURL(DeepLink.inbox)
        case .accessoryRectangular:
            VStack(alignment: .leading, spacing: 2) {
                Label {
                    Text(verbatim: "ClickFix")
                } icon: {
                    Image(systemName: IssueView.symbol(issue))
                }
                .font(.headline)
                .widgetAccentable()
                Text(IssueView.message(issue))
                    .font(.caption)
                    .lineLimit(2)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .widgetURL(DeepLink.inbox)
        default:
            Label {
                Text("Open ClickFix")
            } icon: {
                Image(systemName: IssueView.symbol(issue))
            }
            .widgetURL(DeepLink.inbox)
        }
    }
}

/// Big number with a caption, e.g. "4 Unread".
struct CountView: View {
    let value: Int
    let label: LocalizedStringKey
    var emphasized = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(value, format: .number)
                .font(.system(.largeTitle, design: .rounded, weight: .semibold))
                .monospacedDigit()
                .lineLimit(1)
                .minimumScaleFactor(0.5)
                .foregroundStyle(emphasized ? Brand.signalText : Color.primary)
                .contentTransition(.numericText())
                .widgetAccentable(emphasized)
            Text(label)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
        }
    }
}

/// Inbox row: channel symbol, contact, time, preview (privacy sensitive).
struct ConversationRow: View {
    let item: InboxItem
    let now: Date

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: ChannelSymbol.name(for: item.channel))
                .font(.caption)
                .foregroundStyle(Brand.signalText)
                .frame(width: 16)
                .padding(.top, 2)
                .widgetAccentable()
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 4) {
                    Text(WidgetFormat.contact(item.contact))
                        .font(.subheadline.weight(item.unread > 0 ? .semibold : .regular))
                        .lineLimit(1)
                    Spacer(minLength: 4)
                    if let at = item.at {
                        Text(WidgetFormat.shortTime(at, now: now))
                            .font(.caption2)
                            .monospacedDigit()
                            .foregroundStyle(.secondary)
                    }
                }
                Text(previewText)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .privacySensitive()
            }
            if item.unread > 0 {
                Circle()
                    .fill(Brand.signal)
                    .frame(width: 7, height: 7)
                    .padding(.top, 6)
                    .widgetAccentable()
                    .accessibilityLabel(Text("Unread"))
            }
        }
    }

    private var previewText: String {
        switch item.direction {
        case "out": return String(localized: "You: \(item.preview)")
        case "internal": return String(localized: "Note: \(item.preview)")
        default: return item.preview
        }
    }
}

/// SLA countdown: time left (timer) or time since breach.
struct SLACountdown: View {
    let item: SLAItem
    let now: Date

    var body: some View {
        if let due = item.dueAt, due > now, !item.breached {
            Text(timerInterval: now...due, countsDown: true)
                .monospacedDigit()
        } else if let due = item.dueAt, due <= now {
            // Counts up from the breach, labelled by the caller.
            Text(due, style: .timer)
                .monospacedDigit()
        } else {
            Text("Breached")
        }
    }

    static func tint(_ item: SLAItem, now: Date) -> Color {
        if item.isBreached(at: now) { return .red }
        if let due = item.dueAt, due.timeIntervalSince(now) < 15 * 60 { return .orange }
        return .secondary
    }

    static func symbol(_ item: SLAItem, now: Date) -> String {
        if item.isBreached(at: now) { return "exclamationmark.triangle.fill" }
        if let due = item.dueAt, due.timeIntervalSince(now) < 15 * 60 { return "exclamationmark.circle.fill" }
        return "clock"
    }
}
