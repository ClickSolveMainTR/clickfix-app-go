// CF-152: Siri / Shortcuts / Spotlight phrases. Only the main app may
// declare an AppShortcutsProvider, so this compiles under CLICKFIX_APP (a
// Swift condition the config plugin sets on the app target only).
// Phrase translations: <lang>.lproj/AppShortcuts.strings (keys use ${applicationName}).

#if CLICKFIX_APP
import AppIntents

@available(iOS 17.0, *)
struct ClickFixShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: SetPresenceIntent(),
            phrases: [
                "Set my \(.applicationName) status to \(\.$status)",
                "Change my status in \(.applicationName)",
            ],
            shortTitle: "Set status",
            systemImageName: "person.crop.circle.badge.checkmark"
        )
        AppShortcut(
            intent: OpenUnreadIntent(),
            phrases: [
                "Open my unread conversations in \(.applicationName)",
                "Show my \(.applicationName) inbox",
            ],
            shortTitle: "Unread conversations",
            systemImageName: "tray.full"
        )
        AppShortcut(
            intent: TodayAppointmentsIntent(),
            phrases: [
                "Show today's appointments in \(.applicationName)",
                "What's on my \(.applicationName) agenda today",
            ],
            shortTitle: "Today's appointments",
            systemImageName: "calendar"
        )
        AppShortcut(
            intent: SearchIntent(),
            phrases: [
                "Search in \(.applicationName)",
                "Search \(.applicationName)",
            ],
            shortTitle: "Search",
            systemImageName: "magnifyingglass"
        )
        AppShortcut(
            intent: NewConversationIntent(),
            phrases: [
                "Start a new conversation in \(.applicationName)",
                "New \(.applicationName) conversation",
            ],
            shortTitle: "New conversation",
            systemImageName: "square.and.pencil"
        )
    }
}
#endif
