// CF-152: a tapped Spotlight result (indexed by `indexSpotlight`) arrives as
// a CSSearchableItemActionType user activity whose identifier is the
// `clickfix://` URL. Opening our own URL routes it through expo-linking /
// expo-router like any other deep link, including on cold start.

import CoreSpotlight
import ExpoModulesCore
import UIKit

public class ClickFixWidgetBridgeAppDelegate: ExpoAppDelegateSubscriber {
    public func application(
        _ application: UIApplication,
        continue userActivity: NSUserActivity,
        restorationHandler: @escaping ([any UIUserActivityRestoring]?) -> Void
    ) -> Bool {
        guard userActivity.activityType == CSSearchableItemActionType,
              let identifier = userActivity.userInfo?[CSSearchableItemActivityIdentifier] as? String,
              let url = URL(string: identifier),
              url.scheme?.lowercased() == "clickfix"
        else { return false }

        DispatchQueue.main.async {
            UIApplication.shared.open(url, options: [:]) { opened in
                // Fallback: the app reads the pending route on foreground.
                if !opened, let route = ClickFixWidgetBridgeAppDelegate.route(from: url) {
                    WidgetBridgeStore.setPendingRoute(route)
                }
            }
        }
        restorationHandler(nil)
        return true
    }

    private static func route(from url: URL) -> String? {
        var route = url.path.isEmpty ? "/" : url.path
        if let query = url.query, !query.isEmpty { route += "?" + query }
        return route
    }
}
