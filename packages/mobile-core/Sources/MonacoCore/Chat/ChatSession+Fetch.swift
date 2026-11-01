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

extension ChatSession {
    func catchUp() async {
        guard let start = cursor.id else {
            await loadNewest()
            return
        }
        cursor.begin()
        let caughtUp = await fetchMissed(after: start)
        cursor.finish(caughtUp: caughtUp)
    }

    private func fetchMissed(after start: String) async -> Bool {
        var after = start
        while !Task.isCancelled {
            do {
                let page = try await fetch(after: after)
                state.timeline.mergeNewer(page)
                cursor.advance(with: page)
                adoptSeen(from: page)
                state.isClosed = false
                publish()
                guard page.count >= Self.pageSize,
                    let newest = page.max(by: ChatTimeline.chronological)?.id,
                    newest != after
                else {
                    await loadNewest()
                    return true
                }
                after = newest
            } catch {
                fail(error, firstLoad: false)
                publish()
                return false
            }
        }
        return false
    }
}

struct CatchUpCursor {
    private var newest: ChatMessage?
    private var running = 0
    private var arrived: [ChatMessage] = []

    var id: String? { newest?.id }

    mutating func advance(with messages: [ChatMessage]) {
        newest = ((newest.map { [$0] } ?? []) + messages).max(by: ChatTimeline.chronological)
    }

    mutating func noteLive(_ message: ChatMessage) {
        if running > 0 { arrived.append(message) } else { advance(with: [message]) }
    }

    mutating func begin() {
        running += 1
    }

    mutating func finish(caughtUp: Bool) {
        running -= 1
        guard running == 0 else { return }
        let live = arrived
        arrived = []
        if caughtUp { advance(with: live) }
    }
}
