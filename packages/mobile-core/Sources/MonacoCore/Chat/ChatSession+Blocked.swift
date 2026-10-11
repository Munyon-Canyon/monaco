import MonacoAPI

extension ChatSession {
    public func refreshBlocked() async {
        await blocked?.refresh()
    }

    func isBlocked(_ message: ChatMessage) async -> Bool {
        await blocked?.contains(message.author.id) ?? false
    }

    static func isClosed(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error else { return false }
        return problem.code == .known(.notCabalMember)
    }
}
