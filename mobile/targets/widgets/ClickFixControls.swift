// CF-152: iOS 18 Control Center / Lock Screen / Action button controls.
// Their intents live in `_shared` so they also exist in the app (required
// for intents that open the app).

import AppIntents
import SwiftUI
import WidgetKit

/// Toggle: on = busy, off = online.
@available(iOS 18.0, *)
struct PresenceControl: ControlWidget {
    static let kind = "ClickFixPresenceControl"

    var body: some ControlWidgetConfiguration {
        StaticControlConfiguration(kind: Self.kind, provider: BusyValueProvider()) { isBusy in
            ControlWidgetToggle("Busy", isOn: isBusy, action: SetBusyIntent()) { on in
                Label(
                    on ? LocalizedStringKey("Busy") : LocalizedStringKey("Online"),
                    systemImage: on ? "minus.circle.fill" : "checkmark.circle.fill"
                )
            }
        }
        .displayName("Busy")
        .description("Mark yourself busy in ClickFix.")
    }
}

@available(iOS 18.0, *)
struct BusyValueProvider: ControlValueProvider {
    var previewValue: Bool { false }

    func currentValue() async throws -> Bool {
        SharedStore.effectivePresence(for: SharedStore.readySnapshot()) == .busy
    }
}

@available(iOS 18.0, *)
struct NewConversationControl: ControlWidget {
    static let kind = "ClickFixNewConversationControl"

    var body: some ControlWidgetConfiguration {
        StaticControlConfiguration(kind: Self.kind) {
            ControlWidgetButton(action: OpenNewConversationControlIntent()) {
                Label("New conversation", systemImage: "square.and.pencil")
            }
        }
        .displayName("New conversation")
        .description("Start a new ClickFix conversation.")
    }
}
