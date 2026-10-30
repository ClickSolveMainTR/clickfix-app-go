// CF-152: how intents that open the app reach a screen.
//
// Choice: every "open" intent sets `openAppWhenRun = true` and lives in
// `_shared`, so the system foregrounds the app and runs `perform()` in the
// app process (iOS 17 and 18 alike; Apple requires the intent in both
// targets for this, https://developer.apple.com/forums/thread/763851).
// In the app we open our own `clickfix://` URL, which goes through
// expo-linking / expo-router exactly like a tapped link, including on cold
// start (expo-linking keeps it as the initial URL). If that fails, or the
// code somehow runs in the extension, the route is left in the App Group
// (`pendingRoute`) and the app navigates on its next foreground.
// We do not do both: that would push the screen twice.

import Foundation
#if CLICKFIX_APP
import UIKit
#endif

enum AppRouter {
    @MainActor
    static func open(_ route: String) async {
        #if CLICKFIX_APP
        if await UIApplication.shared.open(DeepLink.url(route)) {
            return
        }
        #endif
        SharedStore.setPendingRoute(route)
    }
}
