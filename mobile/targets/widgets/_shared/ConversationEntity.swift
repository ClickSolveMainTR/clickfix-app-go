// CF-152: conversations as App Entities (Siri, Shortcuts, Spotlight on
// iOS 18). Backed by the snapshot only: no network in queries.

import AppIntents
import Foundation

@available(iOS 17.0, *)
struct ConversationEntity: AppEntity {
    static let typeDisplayRepresentation = TypeDisplayRepresentation(name: "Conversation")
    static let defaultQuery = ConversationQuery()

    let id: String
    let contact: String
    let preview: String
    let channel: String

    var displayRepresentation: DisplayRepresentation {
        DisplayRepresentation(
            title: "\(contact)",
            subtitle: "\(preview)",
            image: .init(systemName: ChannelSymbol.name(for: channel))
        )
    }
}

@available(iOS 17.0, *)
struct ConversationQuery: EntityStringQuery {
    func entities(for identifiers: [ConversationEntity.ID]) async throws -> [ConversationEntity] {
        let all = Self.snapshotEntities()
        return identifiers.compactMap { id in all.first { $0.id == id } }
    }

    func entities(matching string: String) async throws -> [ConversationEntity] {
        let needle = string.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !needle.isEmpty else { return Self.snapshotEntities() }
        return Self.snapshotEntities().filter {
            $0.contact.localizedCaseInsensitiveContains(needle) || $0.preview.localizedCaseInsensitiveContains(needle)
        }
    }

    func suggestedEntities() async throws -> [ConversationEntity] {
        Self.snapshotEntities()
    }

    /// Inbox top first, then SLA items not already listed.
    static func snapshotEntities() -> [ConversationEntity] {
        guard let snapshot = SharedStore.readySnapshot() else { return [] }
        var seen = Set<String>()
        var result: [ConversationEntity] = []
        for item in snapshot.inbox?.top ?? [] where seen.insert(item.uuid).inserted {
            result.append(ConversationEntity(id: item.uuid, contact: item.contact, preview: item.preview, channel: item.channel))
        }
        for item in snapshot.sla where seen.insert(item.uuid).inserted {
            result.append(ConversationEntity(id: item.uuid, contact: item.contact, preview: item.subject, channel: item.channel))
        }
        return result
    }
}

// iOS 18: lets the system index conversations for Spotlight through App
// Intents. The default `attributeSet` (from displayRepresentation) is enough.
@available(iOS 18.0, *)
extension ConversationEntity: IndexedEntity {}
