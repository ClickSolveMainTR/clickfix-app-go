package expo.modules.clickfixwidgetbridge

import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

// CF-152: Android widgets use react-native-android-widget; this module only
// exists so the JS API has the same shape on both platforms. Every call is a
// no-op and isAvailable() is false.
class ClickFixWidgetBridgeModule : Module() {
  override fun definition() = ModuleDefinition {
    Name("ClickFixWidgetBridge")

    Function("isAvailable") { false }

    Function("writeSnapshot") { _: String -> }

    Function("readPresenceOverride") { null as String? }

    Function("setCredential") { _: String? -> }

    Function("consumePendingRoute") { null as String? }

    Function("reloadAll") { }

    AsyncFunction("indexSpotlight") { _: List<Map<String, Any?>> -> }

    AsyncFunction("clearSpotlight") { }

    // Live Activities are iOS only.
    Events("onActivityPushToken", "onPushToStartToken")

    Function("liveActivitiesEnabled") { false }

    Function("startCallActivity") { _: Map<String, Any?>, _: Map<String, Any?> -> null as String? }

    AsyncFunction("updateCallActivity") { _: String, _: Map<String, Any?> -> }

    Function("startUrgentActivity") { _: Map<String, Any?>, _: Map<String, Any?>, _: Double? -> null as String? }

    AsyncFunction("updateUrgentActivity") { _: String, _: Map<String, Any?>, _: Double? -> }

    AsyncFunction("endActivity") { _: String, _: Boolean -> }

    AsyncFunction("endAllActivities") { }

    Function("listActivities") { emptyList<Map<String, String>>() }

    Function("startObservingTokens") { }
  }
}
