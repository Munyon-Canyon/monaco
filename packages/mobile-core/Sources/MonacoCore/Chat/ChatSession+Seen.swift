import MonacoAPI

extension ChatSession {
    public struct Seen: Equatable, Sendable {
        public let messageID: String
        public let count: Int
    }

    func adoptSeen(from page: [ChatMessage]) {
        guard let newest = state.timeline.newestID,
            let count = page.first(where: { $0.id == newest })?.seenCount
        else { return }
        state.seen = Seen(messageID: newest, count: count)
    }
}

extension ChatSession.State {
    public var seenLabel: String? { seen.flatMap { ChatSeenCopy.label(count: $0.count) } }
}
