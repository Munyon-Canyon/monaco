import MonacoAPI

extension ChatSession {
    func fetch(before: String? = nil, after: String? = nil) async throws -> [ChatMessage] {
        let cabalID = cabalID
        return try await api.read { client in
            try await client.getChatMessages(
                path: .init(id: cabalID),
                query: .init(before: before, after: after, limit: Self.pageSize)
            ).ok.body.json.messages
        }
    }
}

struct CatchUpCursor {
    private var newest: ChatMessage?

    var id: String? { newest?.id }

    mutating func advance(with messages: [ChatMessage]) {
        newest = ((newest.map { [$0] } ?? []) + messages).max(by: ChatTimeline.chronological)
    }
}
