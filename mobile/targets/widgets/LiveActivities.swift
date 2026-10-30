// CF-152: Live Activities + Dynamic Island for calls and urgent (SLA)
// conversations. Started / updated / ended by the app through the
// clickfix-widget-bridge module, or by APNs (push-to-start / update).

import ActivityKit
import SwiftUI
import WidgetKit

/// Grouped so the main bundle stays well under WidgetBundleBuilder's
/// 10-component limit.
struct ClickFixLiveActivities: WidgetBundle {
    var body: some Widget {
        CallLiveActivity()
        UrgentConversationLiveActivity()
    }
}

private enum ActivityLinks {
    static let mute = DeepLink.url("/call/mute")
    static let end = DeepLink.url("/call/end")

    static func video(_ room: String) -> URL { DeepLink.url("/video/" + DeepLink.sanitizedId(room)) }
}

private extension Date {
    init(unixSeconds: Double) {
        self = Date(timeIntervalSince1970: unixSeconds)
    }
}

// MARK: - Call

struct CallLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: CallActivityAttributes.self) { context in
            CallLockScreenView(context: context)
                .activityBackgroundTint(Brand.background)
                .activitySystemActionForegroundColor(Color.primary)
                .widgetURL(ActivityLinks.video(context.attributes.room))
        } dynamicIsland: { context in
            DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Label {
                        Text(CallViews.kindTitle(context.state.kind))
                    } icon: {
                        Image(systemName: CallViews.symbol(context.state.kind))
                            .foregroundStyle(Brand.signal)
                    }
                    .font(.caption.weight(.semibold))
                    .lineLimit(1)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    CallViews.timer(context.attributes.startedAt)
                        .font(.body.weight(.semibold))
                        .multilineTextAlignment(.trailing)
                        .frame(maxWidth: 72, alignment: .trailing)
                }
                DynamicIslandExpandedRegion(.center) {
                    VStack(spacing: 0) {
                        Text(context.attributes.title)
                            .font(.headline)
                            .lineLimit(1)
                        if !context.attributes.subtitle.isEmpty {
                            Text(context.attributes.subtitle)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                        }
                    }
                }
                DynamicIslandExpandedRegion(.bottom) {
                    CallViews.actions(muted: context.state.muted)
                }
            } compactLeading: {
                Image(systemName: CallViews.symbol(context.state.kind))
                    .foregroundStyle(Brand.signal)
            } compactTrailing: {
                CallViews.timer(context.attributes.startedAt)
                    .multilineTextAlignment(.trailing)
                    .frame(maxWidth: 44)
            } minimal: {
                Image(systemName: context.state.muted ? "mic.slash.fill" : CallViews.symbol(context.state.kind))
                    .foregroundStyle(Brand.signal)
            }
            .widgetURL(ActivityLinks.video(context.attributes.room))
            .keylineTint(Brand.signal)
        }
    }
}

private enum CallViews {
    static func symbol(_ kind: String) -> String {
        kind == "video" ? "video.fill" : "phone.fill"
    }

    static func kindTitle(_ kind: String) -> LocalizedStringKey {
        kind == "video" ? "Video call" : "Voice call"
    }

    /// Counts up from the start of the call.
    static func timer(_ startedAt: Double) -> some View {
        Text(timerInterval: Date(unixSeconds: startedAt)...Date.distantFuture, countsDown: false)
            .monospacedDigit()
    }

    static func actions(muted: Bool) -> some View {
        HStack(spacing: 10) {
            Link(destination: ActivityLinks.mute) {
                Label(
                    muted ? LocalizedStringKey("Unmute") : LocalizedStringKey("Mute"),
                    systemImage: muted ? "mic.slash.fill" : "mic.fill"
                )
                .font(.subheadline.weight(.semibold))
                .frame(maxWidth: .infinity, minHeight: 36)
                .background(Capsule().fill(Color.primary.opacity(0.12)))
            }
            Link(destination: ActivityLinks.end) {
                Label("End", systemImage: "phone.down.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Color.white)
                    .frame(maxWidth: .infinity, minHeight: 36)
                    .background(Capsule().fill(Color.red))
            }
        }
    }
}

private struct CallLockScreenView: View {
    let context: ActivityViewContext<CallActivityAttributes>

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .center, spacing: 10) {
                Image(systemName: CallViews.symbol(context.state.kind))
                    .font(.title3)
                    .foregroundStyle(Brand.signalText)
                    .frame(width: 28, height: 28)
                    .overlay(alignment: .bottomTrailing) {
                        ClickFixBadge(size: 13).offset(x: 5, y: 5)
                    }
                VStack(alignment: .leading, spacing: 1) {
                    Text(context.attributes.title)
                        .font(.headline)
                        .lineLimit(1)
                    HStack(spacing: 4) {
                        Text(CallViews.kindTitle(context.state.kind))
                        if context.state.muted {
                            Text(verbatim: "·")
                            Text("Muted")
                        }
                    }
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                }
                Spacer(minLength: 8)
                CallViews.timer(context.attributes.startedAt)
                    .font(.title3.weight(.semibold))
                    .multilineTextAlignment(.trailing)
                    .frame(maxWidth: 90, alignment: .trailing)
            }
            CallViews.actions(muted: context.state.muted)
        }
        .padding(16)
    }
}

// MARK: - Urgent conversation

struct UrgentConversationLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: UrgentConversationAttributes.self) { context in
            UrgentLockScreenView(context: context)
                .activityBackgroundTint(Brand.background)
                .activitySystemActionForegroundColor(Color.primary)
                .widgetURL(DeepLink.conversation(context.attributes.conversation))
        } dynamicIsland: { context in
            let tint = UrgentViews.tint(context.state, isStale: context.isStale)
            return DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Label {
                        Text(WidgetFormat.contact(context.state.contact))
                    } icon: {
                        Image(systemName: ChannelSymbol.name(for: context.state.channel))
                            .foregroundStyle(tint)
                    }
                    .font(.caption.weight(.semibold))
                    .lineLimit(1)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    UrgentViews.countdown(context.state)
                        .font(.body.weight(.semibold))
                        .foregroundStyle(tint)
                        .multilineTextAlignment(.trailing)
                        .frame(maxWidth: 72, alignment: .trailing)
                }
                DynamicIslandExpandedRegion(.center) {
                    if !context.state.subject.isEmpty {
                        Text(context.state.subject)
                            .font(.subheadline.weight(.semibold))
                            .lineLimit(1)
                    }
                }
                DynamicIslandExpandedRegion(.bottom) {
                    HStack(spacing: 10) {
                        Text(context.state.preview)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                            .privacySensitive()
                        Spacer(minLength: 4)
                        UrgentViews.replyLink(context.attributes.conversation)
                    }
                }
            } compactLeading: {
                Image(systemName: ChannelSymbol.name(for: context.state.channel))
                    .foregroundStyle(tint)
            } compactTrailing: {
                UrgentViews.countdown(context.state)
                    .foregroundStyle(tint)
                    .multilineTextAlignment(.trailing)
                    .frame(maxWidth: 44)
            } minimal: {
                Image(systemName: UrgentViews.isBreached(context.state) ? "exclamationmark.triangle.fill" : "timer")
                    .foregroundStyle(tint)
            }
            .widgetURL(DeepLink.conversation(context.attributes.conversation))
            .keylineTint(tint)
        }
    }
}

private enum UrgentViews {
    static func isBreached(_ state: UrgentConversationAttributes.ContentState) -> Bool {
        state.breached || Date(unixSeconds: state.dueAt) <= Date()
    }

    /// Red when breached or stale, orange under 15 minutes, signal otherwise.
    static func tint(_ state: UrgentConversationAttributes.ContentState, isStale: Bool) -> Color {
        if isBreached(state) || isStale { return .red }
        if Date(unixSeconds: state.dueAt).timeIntervalSinceNow < 15 * 60 { return .orange }
        return Brand.signal
    }

    /// Time left until the SLA target, or time since it was missed.
    @ViewBuilder
    static func countdown(_ state: UrgentConversationAttributes.ContentState) -> some View {
        let due = Date(unixSeconds: state.dueAt)
        let now = Date()
        if !state.breached, due > now {
            Text(timerInterval: now...due, countsDown: true)
                .monospacedDigit()
        } else {
            Text(due, style: .timer)
                .monospacedDigit()
        }
    }

    static func replyLink(_ conversation: String) -> some View {
        Link(destination: DeepLink.conversation(conversation)) {
            Label("Reply", systemImage: "arrowshape.turn.up.left.fill")
                .font(.subheadline.weight(.semibold))
                .padding(.horizontal, 14)
                .padding(.vertical, 8)
                .foregroundStyle(Color.black)
                .background(Capsule().fill(Brand.signal))
        }
    }
}

private struct UrgentLockScreenView: View {
    let context: ActivityViewContext<UrgentConversationAttributes>

    var body: some View {
        let state = context.state
        let tint = UrgentViews.tint(state, isStale: context.isStale)
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .center, spacing: 10) {
                Image(systemName: ChannelSymbol.name(for: state.channel))
                    .font(.title3)
                    .foregroundStyle(tint)
                    .frame(width: 28, height: 28)
                    .overlay(alignment: .bottomTrailing) {
                        ClickFixBadge(size: 13).offset(x: 5, y: 5)
                    }
                VStack(alignment: .leading, spacing: 1) {
                    Text(WidgetFormat.contact(state.contact))
                        .font(.headline)
                        .lineLimit(1)
                    if !state.subject.isEmpty {
                        Text(state.subject)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                Spacer(minLength: 8)
                VStack(alignment: .trailing, spacing: 0) {
                    Text(UrgentViews.isBreached(state) ? LocalizedStringKey("Breached") : LocalizedStringKey("Due in"))
                        .font(.caption2.weight(.semibold))
                        .foregroundStyle(tint)
                    UrgentViews.countdown(state)
                        .font(.title3.weight(.semibold))
                        .foregroundStyle(tint)
                        .multilineTextAlignment(.trailing)
                        .frame(maxWidth: 90, alignment: .trailing)
                }
            }
            HStack(alignment: .center, spacing: 10) {
                Text(state.preview)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                    .privacySensitive()
                Spacer(minLength: 8)
                UrgentViews.replyLink(context.attributes.conversation)
            }
        }
        .padding(16)
    }
}
