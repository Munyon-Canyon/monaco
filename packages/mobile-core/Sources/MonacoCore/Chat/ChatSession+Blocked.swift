import MonacoAPI

extension ChatSession {
    public func refreshBlocked() async {
        guard let blocked else { return }
        do {
            try await blocked.refresh()
        } catch {
            if !Task.isCancelled { fail(error, firstLoad: false) }
        }
        let authors = await blocked.authors
        state.timeline.removeMessages(by: authors)
        for thread in attached.values { await thread.dropBlocked(authors) }
        publish()
    }

    func isBlocked(_ message: ChatMessage) async -> Bool {
        await blocked?.contains(message.author.id) ?? false
    }

    static func isClosed(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error else { return false }
        return problem.code == .known(.notCabalMember)
    }
}
