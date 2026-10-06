import Foundation
import MonacoAPI

public typealias ChatMessage = Components.Schemas.ChatMessage

public struct ChatRow: Identifiable, Equatable, Sendable {
    public enum Delivery: Equatable, Sendable {
        case sent
        case pending
        case failed
    }

    public let message: ChatMessage
    public let parentBody: String?
    public let delivery: Delivery
    public let isMine: Bool
    public let startsDay: Bool
    public let startsRun: Bool
    public let endsRun: Bool

    public var id: String { message.id }
    public var date: Date { message.createdAt }
    public var isPlaceholder: Bool { message.deleted }

    public func dayLabel(now: Date, calendar: Calendar = .current, locale: Locale = .current) -> String? {
        guard startsDay else { return nil }
        return ChatDayLabel.text(for: date, now: now, calendar: calendar, locale: locale)
    }
}

public enum ChatDayLabel {
    public static func text(for date: Date, now: Date, calendar: Calendar = .current, locale: Locale = .current)
        -> String
    {
        if calendar.isDate(date, inSameDayAs: now) { return "Today" }
        if let yesterday = calendar.date(byAdding: .day, value: -1, to: now),
            calendar.isDate(date, inSameDayAs: yesterday)
        {
            return "Yesterday"
        }
        let sameYear = calendar.component(.year, from: date) == calendar.component(.year, from: now)
        return SharedFormatters.string(
            from: date,
            pattern: .template(sameYear ? "MMMd" : "yMMMd"),
            locale: locale,
            calendar: calendar
        )
    }
}

public struct ChatTimeline: Equatable, Sendable {
    struct Unsent: Equatable, Sendable {
        let key: String
        let body: String
        let createdAt: Date
        let parentID: String?
        let alsoInChannel: Bool
        var failed: Bool
    }

    public enum Scope: Equatable, Sendable {
        case channel
        case thread(parentID: String)
    }

    public let viewerID: String
    public let scope: Scope
    public private(set) var messages: [ChatMessage] = []
    public private(set) var rows: [ChatRow] = []
    public private(set) var hasLoadedNewest = false
    public private(set) var hasOlder = false
    private(set) var unsent: [Unsent] = []

    public init(viewerID: String, scope: Scope = .channel) {
        self.viewerID = viewerID
        self.scope = scope
    }

    public var newestID: String? { messages.last?.id }
    public var oldestID: String? { messages.first?.id }

    public func accepts(_ message: ChatMessage) -> Bool {
        switch scope {
        case .channel: message.parentId == nil || message.alsoInChannel
        case .thread(let parentID): message.parentId == parentID
        }
    }

    public func message(id: String) -> ChatMessage? {
        messages.first { $0.id == id }
    }

    public mutating func restore(_ message: ChatMessage) {
        _ = upsert([message])
    }

    @discardableResult
    public mutating func mergeNewest(_ page: [ChatMessage], pageSize: Int) -> [ChatMessage] {
        if !hasLoadedNewest { hasOlder = page.count >= pageSize }
        hasLoadedNewest = true
        return upsert(page)
    }

    @discardableResult
    public mutating func mergeNewer(_ page: [ChatMessage]) -> [ChatMessage] {
        hasLoadedNewest = true
        return upsert(page)
    }

    @discardableResult
    public mutating func mergeOlder(_ page: [ChatMessage], pageSize: Int) -> [ChatMessage] {
        hasOlder = page.count >= pageSize
        return upsert(page)
    }

    @discardableResult
    public mutating func insertLive(_ message: ChatMessage) -> Bool {
        guard accepts(message), !messages.contains(where: { $0.id == message.id }) else { return false }
        dropEchoed(by: message)
        messages.append(message)
        messages.sort(by: Self.chronological)
        rebuildRows()
        return true
    }

    public mutating func settle(_ message: ChatMessage, key: String) {
        unsent.removeAll { $0.key == key }
        if let index = messages.firstIndex(where: { $0.id == message.id }) {
            messages[index] = message
        } else {
            messages.append(message)
            messages.sort(by: Self.chronological)
        }
        rebuildRows()
    }

    public mutating func markDeleted(id: String) {
        guard let index = messages.firstIndex(where: { $0.id == id }) else { return }
        if messages[index].replyCount > 0 {
            messages[index].deleted = true
            messages[index].body = nil
        } else {
            messages.remove(at: index)
        }
        rebuildRows()
    }

    public mutating func applyThread(id: String, replyCount: Int, lastReplyAt: Date?) {
        guard let index = messages.firstIndex(where: { $0.id == id }) else { return }
        messages[index].replyCount = replyCount
        messages[index].lastReplyAt = lastReplyAt
        rebuildRows()
    }

    mutating func addUnsent(
        key: String, body: String, at createdAt: Date, parentID: String? = nil, alsoInChannel: Bool = false
    ) {
        unsent.append(
            Unsent(
                key: key, body: body, createdAt: createdAt, parentID: parentID, alsoInChannel: alsoInChannel,
                failed: false))
        rebuildRows()
    }

    mutating func setFailed(key: String, _ failed: Bool) {
        guard let index = unsent.firstIndex(where: { $0.key == key }) else { return }
        unsent[index].failed = failed
        rebuildRows()
    }

    mutating func dropUnsent(key: String) {
        unsent.removeAll { $0.key == key }
        rebuildRows()
    }

    func unsent(key: String) -> Unsent? {
        unsent.first { $0.key == key }
    }

    private mutating func dropEchoed(by message: ChatMessage) {
        guard message.author.id == viewerID,
            let index = unsent.firstIndex(where: { !$0.failed && $0.body == message.body })
        else { return }
        unsent.remove(at: index)
    }

    private mutating func upsert(_ incoming: [ChatMessage]) -> [ChatMessage] {
        var added: [ChatMessage] = []
        var changed = false
        for message in incoming where accepts(message) {
            if let index = messages.firstIndex(where: { $0.id == message.id }) {
                if messages[index] != message {
                    messages[index] = message
                    changed = true
                }
            } else {
                messages.append(message)
                added.append(message)
                changed = true
            }
        }
        guard changed else { return [] }
        messages.sort(by: Self.chronological)
        rebuildRows()
        return added.sorted(by: Self.chronological)
    }

    private mutating func rebuildRows() {
        let local = unsent.map { Self.message(for: $0, viewerID: viewerID) }
        let all = messages + local
        let calendar = Calendar.current
        rows = all.indices.map { index in
            let message = all[index]
            let previous = index > 0 ? all[index - 1] : nil
            let next = index + 1 < all.count ? all[index + 1] : nil
            let startsDay = previous.map { !calendar.isDate($0.createdAt, inSameDayAs: message.createdAt) } ?? true
            let nextStartsDay = next.map { !calendar.isDate(message.createdAt, inSameDayAs: $0.createdAt) } ?? false
            return ChatRow(
                message: message,
                parentBody: message.parentId.flatMap { id in messages.first { $0.id == id }?.body },
                delivery: delivery(of: message),
                isMine: message.author.id == viewerID,
                startsDay: startsDay,
                startsRun: startsDay || previous?.author.id != message.author.id,
                endsRun: next == nil || nextStartsDay || next?.author.id != message.author.id
            )
        }
    }

    private func delivery(of message: ChatMessage) -> ChatRow.Delivery {
        guard let send = unsent.first(where: { $0.key == message.id }) else { return .sent }
        return send.failed ? .failed : .pending
    }

    private static func message(for send: Unsent, viewerID: String) -> ChatMessage {
        ChatMessage(
            id: send.key,
            author: .init(id: viewerID, handle: nil, displayName: "", photoUrl: nil),
            body: send.body,
            createdAt: send.createdAt,
            parentId: send.parentID,
            alsoInChannel: send.alsoInChannel,
            replyCount: 0,
            deleted: false
        )
    }

    static func chronological(_ lhs: ChatMessage, _ rhs: ChatMessage) -> Bool {
        if lhs.createdAt != rhs.createdAt { return lhs.createdAt < rhs.createdAt }
        return lhs.id < rhs.id
    }
}
