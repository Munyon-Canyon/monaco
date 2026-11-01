extension APIClient {
    public func readAllBlockedUsers() async throws -> [Components.Schemas.BlockedUser] {
        var users: [Components.Schemas.BlockedUser] = []
        var cursor: String?
        for _ in 0..<Self.blockedPageLimit {
            let after = cursor
            let page = try await read { try await $0.getMeBlocks(query: .init(cursor: after)).ok.body.json }
            users += page.users
            guard let next = page.nextCursor else { return users }
            cursor = next
        }
        return users
    }

    private static let blockedPageLimit = 100
}
