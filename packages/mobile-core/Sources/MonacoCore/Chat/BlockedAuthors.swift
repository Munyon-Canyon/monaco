import MonacoAPI

public actor BlockedAuthors {
    private let api: APIClient
    private var ids: Set<String> = []

    public init(api: APIClient) {
        self.api = api
    }

    public var authors: Set<String> { ids }

    public func contains(_ userID: String) -> Bool { ids.contains(userID) }

    public func refresh() async throws {
        ids = Set(try await api.readAllBlockedUsers().map(\.userId))
    }
}
