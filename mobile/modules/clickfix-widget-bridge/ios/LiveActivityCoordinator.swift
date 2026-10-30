// CF-152: starts / updates / ends the ClickFix Live Activities and reports
// their APNs tokens (per-activity update tokens and the push-to-start token)
// to JS, which registers them with POST /v1/mobile/live-activity-tokens.
// The pod targets iOS 16.4, so 16.2 ActivityKit APIs need no checks;
// push-to-start (17.2) does.

import ActivityKit
import ExpoModulesCore
import Foundation

struct CallAttributesRecord: Record {
    @Field var room: String = ""
    @Field var title: String = ""
    @Field var subtitle: String = ""
    @Field var startedAt: Double = 0
}

struct CallStateRecord: Record {
    @Field var muted: Bool = false
    @Field var kind: String = "video"
}

struct UrgentAttributesRecord: Record {
    @Field var conversation: String = ""
}

struct UrgentStateRecord: Record {
    @Field var contact: String = ""
    @Field var channel: String = ""
    @Field var subject: String = ""
    @Field var preview: String = ""
    @Field var dueAt: Double = 0
    @Field var breached: Bool = false
}

enum LiveActivityType {
    static let call = "call"
    static let urgent = "urgent_conversation"
}

final class LiveActivityCoordinator {
    typealias Emit = (_ event: String, _ body: [String: Any?]) -> Void

    static let pushTokenEvent = "onActivityPushToken"
    static let pushToStartTokenEvent = "onPushToStartToken"

    private let emit: Emit
    private let lock = NSLock()
    private var observedIds = Set<String>()
    private var lastTokens: [String: [String: Any?]] = [:]
    private var lastPushToStartToken: String?
    private var observing = false
    private var tasks: [Task<Void, Never>] = []

    init(emit: @escaping Emit) {
        self.emit = emit
    }

    deinit {
        tasks.forEach { $0.cancel() }
    }

    var enabled: Bool {
        ActivityAuthorizationInfo().areActivitiesEnabled
    }

    // MARK: Call

    func startCall(_ attributes: CallAttributesRecord, _ state: CallStateRecord) -> String? {
        guard enabled else { return nil }
        // One call at a time: end the others right away.
        let others = Activity<CallActivityAttributes>.activities
        Task {
            for activity in others {
                await activity.end(nil, dismissalPolicy: .immediate)
            }
        }
        let content = ActivityContent(state: Self.callState(state), staleDate: nil)
        let attrs = CallActivityAttributes(
            room: attributes.room,
            title: attributes.title,
            subtitle: attributes.subtitle,
            startedAt: attributes.startedAt
        )
        do {
            let activity = try Activity.request(attributes: attrs, content: content, pushType: .token)
            observeCall(activity)
            return activity.id
        } catch {
            return nil
        }
    }

    func updateCall(id: String, state: CallStateRecord) async {
        guard let activity = Activity<CallActivityAttributes>.activities.first(where: { $0.id == id }) else { return }
        await activity.update(ActivityContent(state: Self.callState(state), staleDate: nil))
    }

    // MARK: Urgent conversation

    func startUrgent(_ attributes: UrgentAttributesRecord, _ state: UrgentStateRecord, staleAt: Double?) -> String? {
        guard enabled else { return nil }
        let content = ActivityContent(state: Self.urgentState(state), staleDate: Self.date(staleAt))
        let attrs = UrgentConversationAttributes(conversation: attributes.conversation)
        do {
            let activity = try Activity.request(attributes: attrs, content: content, pushType: .token)
            observeUrgent(activity)
            return activity.id
        } catch {
            return nil
        }
    }

    func updateUrgent(id: String, state: UrgentStateRecord, staleAt: Double?) async {
        guard let activity = Activity<UrgentConversationAttributes>.activities.first(where: { $0.id == id }) else { return }
        await activity.update(ActivityContent(state: Self.urgentState(state), staleDate: Self.date(staleAt)))
    }

    // MARK: Both types

    func end(id: String, immediate: Bool) async {
        let policy: ActivityUIDismissalPolicy = immediate ? .immediate : .default
        if let call = Activity<CallActivityAttributes>.activities.first(where: { $0.id == id }) {
            await call.end(nil, dismissalPolicy: policy)
        }
        if let urgent = Activity<UrgentConversationAttributes>.activities.first(where: { $0.id == id }) {
            await urgent.end(nil, dismissalPolicy: policy)
        }
        forget(id)
    }

    func endAll() async {
        for activity in Activity<CallActivityAttributes>.activities {
            await activity.end(nil, dismissalPolicy: .immediate)
            forget(activity.id)
        }
        for activity in Activity<UrgentConversationAttributes>.activities {
            await activity.end(nil, dismissalPolicy: .immediate)
            forget(activity.id)
        }
    }

    /// Running activities, including ones started by push-to-start.
    func list() -> [[String: String]] {
        var result: [[String: String]] = []
        for activity in Activity<CallActivityAttributes>.activities where Self.isLive(activity.activityState) {
            result.append(["id": activity.id, "type": LiveActivityType.call, "ref": activity.attributes.room])
        }
        for activity in Activity<UrgentConversationAttributes>.activities where Self.isLive(activity.activityState) {
            result.append(["id": activity.id, "type": LiveActivityType.urgent, "ref": activity.attributes.conversation])
        }
        return result
    }

    // MARK: Token observation

    /// Idempotent. The first call observes existing and future activities
    /// and the push-to-start token; later calls re-emit the last known
    /// tokens (e.g. after signing in again).
    func startObservingTokens() {
        lock.lock()
        let alreadyObserving = observing
        observing = true
        let cachedTokens = Array(lastTokens.values)
        let cachedStartToken = lastPushToStartToken
        lock.unlock()

        if alreadyObserving {
            cachedTokens.forEach { emit(Self.pushTokenEvent, $0) }
            if let cachedStartToken {
                emit(Self.pushToStartTokenEvent, ["type": LiveActivityType.urgent, "token": cachedStartToken])
            }
            return
        }

        Activity<CallActivityAttributes>.activities.forEach(observeCall)
        Activity<UrgentConversationAttributes>.activities.forEach(observeUrgent)

        var started: [Task<Void, Never>] = []
        started.append(Task { [weak self] in
            for await activity in Activity<CallActivityAttributes>.activityUpdates {
                self?.observeCall(activity)
            }
        })
        started.append(Task { [weak self] in
            for await activity in Activity<UrgentConversationAttributes>.activityUpdates {
                self?.observeUrgent(activity)
            }
        })
        if #available(iOS 17.2, *) {
            started.append(Task { [weak self] in
                for await data in Activity<UrgentConversationAttributes>.pushToStartTokenUpdates {
                    self?.publishPushToStart(LiveActivityCoordinator.hex(data))
                }
            })
        }
        lock.lock()
        tasks.append(contentsOf: started)
        lock.unlock()
    }

    private func observeCall(_ activity: Activity<CallActivityAttributes>) {
        observe(activity, type: LiveActivityType.call, ref: activity.attributes.room)
    }

    private func observeUrgent(_ activity: Activity<UrgentConversationAttributes>) {
        observe(activity, type: LiveActivityType.urgent, ref: activity.attributes.conversation)
    }

    private func observe<A: ActivityAttributes>(_ activity: Activity<A>, type: String, ref: String) {
        let id = activity.id
        lock.lock()
        let isNew = observedIds.insert(id).inserted
        lock.unlock()
        guard isNew else { return }

        let task = Task { [weak self] in
            for await data in activity.pushTokenUpdates {
                self?.publishToken(id: id, type: type, ref: ref, token: LiveActivityCoordinator.hex(data))
            }
        }
        lock.lock()
        tasks.append(task)
        lock.unlock()
    }

    private func publishToken(id: String, type: String, ref: String, token: String) {
        let body: [String: Any?] = ["id": id, "type": type, "ref": ref, "token": token]
        lock.lock()
        lastTokens[id] = body
        lock.unlock()
        emit(Self.pushTokenEvent, body)
    }

    private func publishPushToStart(_ token: String) {
        lock.lock()
        lastPushToStartToken = token
        lock.unlock()
        emit(Self.pushToStartTokenEvent, ["type": LiveActivityType.urgent, "token": token])
    }

    private func forget(_ id: String) {
        lock.lock()
        lastTokens[id] = nil
        lock.unlock()
    }

    // MARK: Helpers

    private static func callState(_ state: CallStateRecord) -> CallActivityAttributes.ContentState {
        CallActivityAttributes.ContentState(muted: state.muted, kind: state.kind == "voice" ? "voice" : "video")
    }

    private static func urgentState(_ state: UrgentStateRecord) -> UrgentConversationAttributes.ContentState {
        UrgentConversationAttributes.ContentState(
            contact: state.contact,
            channel: state.channel,
            subject: state.subject,
            preview: state.preview,
            dueAt: state.dueAt,
            breached: state.breached
        )
    }

    private static func date(_ unixSeconds: Double?) -> Date? {
        guard let unixSeconds, unixSeconds > 0 else { return nil }
        return Date(timeIntervalSince1970: unixSeconds)
    }

    private static func isLive(_ state: ActivityState) -> Bool {
        switch state {
        case .active, .stale: return true
        default: return false
        }
    }

    static func hex(_ data: Data) -> String {
        data.map { String(format: "%02x", $0) }.joined()
    }
}
