// CF-152: intents that open a ClickFix screen (see AppRouter for how), plus
// the agenda summary that answers without opening the app.

import AppIntents
import Foundation

@available(iOS 17.0, *)
struct OpenUnreadIntent: AppIntent {
    static let title: LocalizedStringResource = "Open unread conversations"
    static let description = IntentDescription("Opens your ClickFix inbox.")
    static let openAppWhenRun = true

    @MainActor
    func perform() async throws -> some IntentResult {
        await AppRouter.open("/inbox")
        return .result()
    }
}

@available(iOS 17.0, *)
struct OpenAgendaIntent: AppIntent {
    static let title: LocalizedStringResource = "Open agenda"
    static let description = IntentDescription("Opens today's agenda in ClickFix.")
    static let openAppWhenRun = true

    @MainActor
    func perform() async throws -> some IntentResult {
        await AppRouter.open("/agenda")
        return .result()
    }
}

@available(iOS 17.0, *)
struct NewConversationIntent: AppIntent {
    static let title: LocalizedStringResource = "New conversation"
    static let description = IntentDescription("Starts a new conversation in ClickFix.")
    static let openAppWhenRun = true

    @MainActor
    func perform() async throws -> some IntentResult {
        await AppRouter.open("/new-conversation")
        return .result()
    }
}

@available(iOS 17.0, *)
struct OpenAssistantIntent: AppIntent {
    static let title: LocalizedStringResource = "Open assistant"
    static let description = IntentDescription("Opens the ClickFix assistant.")
    static let openAppWhenRun = true

    @MainActor
    func perform() async throws -> some IntentResult {
        await AppRouter.open("/assistant")
        return .result()
    }
}

@available(iOS 17.0, *)
struct SearchIntent: AppIntent {
    static let title: LocalizedStringResource = "Search ClickFix"
    static let description = IntentDescription("Searches conversations, contacts and more in ClickFix.")
    static let openAppWhenRun = true

    // Optional so the widget button can open an empty search screen.
    @Parameter(title: "Search for")
    var query: String?

    init() {}

    init(query: String?) {
        self.query = query
    }

    static var parameterSummary: some ParameterSummary {
        Summary("Search ClickFix for \(\.$query)")
    }

    @MainActor
    func perform() async throws -> some IntentResult {
        await AppRouter.open(DeepLink.searchRoute(query))
        return .result()
    }
}

/// `OpenIntent` so Spotlight / Siri can open a conversation entity directly.
@available(iOS 17.0, *)
struct OpenConversationIntent: OpenIntent {
    static let title: LocalizedStringResource = "Open conversation"
    static let description = IntentDescription("Opens a ClickFix conversation.")
    static let openAppWhenRun = true

    @Parameter(title: "Conversation")
    var target: ConversationEntity

    init() {}

    init(conversation: ConversationEntity) {
        self.target = conversation
    }

    @MainActor
    func perform() async throws -> some IntentResult {
        await AppRouter.open(DeepLink.conversationRoute(target.id))
        return .result()
    }
}

/// Answers "today's appointments" from the snapshot without opening the app.
@available(iOS 17.0, *)
struct TodayAppointmentsIntent: AppIntent {
    static let title: LocalizedStringResource = "Today's appointments"
    static let description = IntentDescription("Tells you what is left on today's agenda.")
    static let openAppWhenRun = false

    func perform() async throws -> some IntentResult & ProvidesDialog {
        guard let snapshot = SharedStore.readySnapshot() else {
            return .result(dialog: "Open ClickFix to see your appointments.")
        }
        guard snapshot.can.appointments else {
            return .result(dialog: "Your role cannot view appointments.")
        }
        let now = Date()
        let upcoming = snapshot.today.filter { $0.isUpcoming(at: now) }
        guard let next = upcoming.first else {
            return .result(dialog: "No more appointments today.")
        }
        let time = next.startsAt.map { $0.formatted(date: .omitted, time: .shortened) } ?? ""
        let title = next.title.isEmpty ? String(localized: "Appointment") : next.title
        if upcoming.count == 1 {
            return .result(dialog: "One appointment left today: \(title) at \(time).")
        }
        return .result(dialog: "\(upcoming.count) appointments left today. Next: \(title) at \(time).")
    }
}

/// iOS 18 control button: the documented OpenURLIntent pattern for controls.
/// `openAppWhenRun` plus presence in both targets lets the system run it.
@available(iOS 18.0, *)
struct OpenNewConversationControlIntent: AppIntent {
    static let title: LocalizedStringResource = "New conversation"
    static let description = IntentDescription("Starts a new conversation in ClickFix.")
    static let openAppWhenRun = true
    static let isDiscoverable = false

    @MainActor
    func perform() async throws -> some IntentResult & OpensIntent {
        .result(opensIntent: OpenURLIntent(DeepLink.url("/new-conversation")))
    }
}
