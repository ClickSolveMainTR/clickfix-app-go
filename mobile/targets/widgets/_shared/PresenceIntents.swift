// CF-152: presence intents. They run in the background (widget button,
// Control Center toggle, Siri) with the widget token; the app is not opened.

import AppIntents
import Foundation

@available(iOS 17.0, *)
enum PresenceStatusOption: String, AppEnum {
    case online
    case away
    case busy
    case offline

    static let typeDisplayRepresentation = TypeDisplayRepresentation(name: "Status")

    static let caseDisplayRepresentations: [PresenceStatusOption: DisplayRepresentation] = [
        .online: DisplayRepresentation(title: "Online", image: .init(systemName: "checkmark.circle.fill")),
        .away: DisplayRepresentation(title: "Away", image: .init(systemName: "moon.fill")),
        .busy: DisplayRepresentation(title: "Busy", image: .init(systemName: "minus.circle.fill")),
        .offline: DisplayRepresentation(title: "Offline", image: .init(systemName: "circle.dashed")),
    ]

    var status: PresenceStatus { PresenceStatus(rawValue: rawValue) ?? .online }
}

@available(iOS 17.0, *)
struct SetPresenceIntent: AppIntent {
    static let title: LocalizedStringResource = "Set status"
    static let description = IntentDescription("Changes your ClickFix availability without opening the app.")
    static let openAppWhenRun = false

    @Parameter(title: "Status")
    var status: PresenceStatusOption

    init() {}

    init(status: PresenceStatusOption) {
        self.status = status
    }

    static var parameterSummary: some ParameterSummary {
        Summary("Set my status to \(\.$status)")
    }

    func perform() async throws -> some IntentResult & ProvidesDialog {
        let target = status.status
        try await PresenceService.apply(target)
        let name = target.localizedName
        return .result(dialog: "Your status is now \(name).")
    }
}

/// Backs the iOS 18 Control Center toggle: on = busy, off = online.
@available(iOS 18.0, *)
struct SetBusyIntent: SetValueIntent {
    static let title: LocalizedStringResource = "Busy"
    static let description = IntentDescription("Turns Busy on or off.")
    static let isDiscoverable = false

    @Parameter(title: "Busy")
    var value: Bool

    func perform() async throws -> some IntentResult {
        try await PresenceService.apply(value ? .busy : .online)
        return .result()
    }
}
