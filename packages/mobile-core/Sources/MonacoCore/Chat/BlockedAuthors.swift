import MonacoAPI

public actor BlockedAuthors {
    private let api: APIClient
    private var ids: Set<String> = []

    public init(api: APIClient) {
        self.api = api
    }

    public func contains(_ userID: String) -> Bool { ids.contains(userID) }

    public func refresh() async {
        guard let page = try? await api.read({ try await $0.getMeBlocks().ok.body.json }) else { return }
        ids = Set(page.users.map(\.userId))
    }
}
