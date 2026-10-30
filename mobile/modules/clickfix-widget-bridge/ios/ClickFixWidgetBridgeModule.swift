// CF-152: JS bridge for the iOS widgets, App Intents and Spotlight.

import CoreSpotlight
import ExpoModulesCore
import Foundation
import UniformTypeIdentifiers

struct SpotlightItemRecord: Record {
    @Field var id: String = ""
    @Field var title: String = ""
    @Field var subtitle: String = ""
    @Field var url: String = ""
    @Field var keywords: [String]? = nil
}

final class KeychainWriteException: GenericException<Int32> {
    override var reason: String {
        "Could not store the widget credential in the shared Keychain (OSStatus \(param))"
    }
}

final class SpotlightException: GenericException<String> {
    override var reason: String {
        "Spotlight indexing failed: \(param)"
    }
}

public class ClickFixWidgetBridgeModule: Module {
    private lazy var liveActivities = LiveActivityCoordinator { [weak self] event, body in
        self?.sendEvent(event, body)
    }

    public func definition() -> ModuleDefinition {
        Name("ClickFixWidgetBridge")

        Events(LiveActivityCoordinator.pushTokenEvent, LiveActivityCoordinator.pushToStartTokenEvent)

        Function("isAvailable") { () -> Bool in
            WidgetBridgeStore.defaults != nil
        }

        Function("writeSnapshot") { (json: String) in
            WidgetBridgeStore.writeSnapshot(json)
        }

        Function("readPresenceOverride") { () -> String? in
            WidgetBridgeStore.readPresenceOverride()
        }

        Function("setCredential") { (json: String?) throws in
            let status = WidgetBridgeStore.setCredential(json)
            if status != errSecSuccess {
                throw KeychainWriteException(status)
            }
        }

        Function("consumePendingRoute") { () -> String? in
            WidgetBridgeStore.consumePendingRoute()
        }

        Function("reloadAll") {
            WidgetBridgeStore.reloadSurfaces()
        }

        AsyncFunction("indexSpotlight") { (items: [SpotlightItemRecord], promise: Promise) in
            let searchable = items.compactMap(ClickFixWidgetBridgeModule.searchableItem)
            let index = CSSearchableIndex.default()
            // Replace the whole domain so conversations that left the list
            // disappear from Spotlight too.
            index.deleteSearchableItems(withDomainIdentifiers: [ClickFixWidgetBridgeModule.spotlightDomain]) { _ in
                guard !searchable.isEmpty else {
                    promise.resolve()
                    return
                }
                index.indexSearchableItems(searchable) { error in
                    if let error {
                        promise.reject(SpotlightException(error.localizedDescription))
                    } else {
                        promise.resolve()
                    }
                }
            }
        }

        // MARK: Live Activities

        Function("liveActivitiesEnabled") { () -> Bool in
            self.liveActivities.enabled
        }

        Function("startCallActivity") { (attributes: CallAttributesRecord, state: CallStateRecord) -> String? in
            self.liveActivities.startCall(attributes, state)
        }

        AsyncFunction("updateCallActivity") { (id: String, state: CallStateRecord) async in
            await self.liveActivities.updateCall(id: id, state: state)
        }

        Function("startUrgentActivity") { (attributes: UrgentAttributesRecord, state: UrgentStateRecord, staleAt: Double?) -> String? in
            self.liveActivities.startUrgent(attributes, state, staleAt: staleAt)
        }

        AsyncFunction("updateUrgentActivity") { (id: String, state: UrgentStateRecord, staleAt: Double?) async in
            await self.liveActivities.updateUrgent(id: id, state: state, staleAt: staleAt)
        }

        AsyncFunction("endActivity") { (id: String, immediate: Bool) async in
            await self.liveActivities.end(id: id, immediate: immediate)
        }

        AsyncFunction("endAllActivities") { () async in
            await self.liveActivities.endAll()
        }

        Function("listActivities") { () -> [[String: String]] in
            self.liveActivities.list()
        }

        Function("startObservingTokens") {
            self.liveActivities.startObservingTokens()
        }

        AsyncFunction("clearSpotlight") { (promise: Promise) in
            CSSearchableIndex.default().deleteSearchableItems(withDomainIdentifiers: [ClickFixWidgetBridgeModule.spotlightDomain]) { error in
                if let error {
                    promise.reject(SpotlightException(error.localizedDescription))
                } else {
                    promise.resolve()
                }
            }
        }
    }

    static let spotlightDomain = "com.clickfix.conversations"

    /// Only `clickfix://` URLs are indexed; the URL is the unique identifier,
    /// which the app delegate subscriber opens when a result is tapped.
    static func searchableItem(_ item: SpotlightItemRecord) -> CSSearchableItem? {
        guard let url = URL(string: item.url), url.scheme?.lowercased() == "clickfix" else { return nil }
        let attributes = CSSearchableItemAttributeSet(contentType: UTType.text)
        attributes.title = item.title
        attributes.contentDescription = item.subtitle
        if let keywords = item.keywords, !keywords.isEmpty {
            attributes.keywords = keywords
        }
        return CSSearchableItem(
            uniqueIdentifier: url.absoluteString,
            domainIdentifier: spotlightDomain,
            attributeSet: attributes
        )
    }
}
