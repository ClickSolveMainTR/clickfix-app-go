// CF-152: entry point of the widget extension. Not in `_shared`: the app
// has its own @main.

import SwiftUI
import WidgetKit

@main
struct ClickFixWidgetBundle: WidgetBundle {
    var body: some Widget {
        InboxWidget()
        SLAWidget()
        TodayWidget()
        PresenceWidget()
        UnreadAccessoryWidget()
        NextAppointmentAccessoryWidget()
        QuickActionsWidget()
        // Live Activities (call, urgent conversation) as a nested bundle.
        ClickFixLiveActivities().body
        // The extension deploys to iOS 17 while controls need iOS 18.
        // WidgetBundleBuilder supports `if #available` (buildLimitedAvailability),
        // the same pattern used to ship iOS 16.1 Live Activities from older
        // bundles; on iOS 17 the controls are simply absent.
        if #available(iOS 18.0, *) {
            PresenceControl()
            NewConversationControl()
        }
    }
}
