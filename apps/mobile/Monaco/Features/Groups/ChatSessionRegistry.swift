import MonacoCore

@MainActor
enum ChatSessionRegistry {
    private struct Entry {
        weak var session: ChatSession?
    }

    private static var entries: [String: Entry] = [:]

    static func register(_ session: ChatSession) {
        entries[session.cabalID] = Entry(session: session)
    }

    static func session(for cabalID: String) -> ChatSession? {
        entries[cabalID]?.session
    }
}
