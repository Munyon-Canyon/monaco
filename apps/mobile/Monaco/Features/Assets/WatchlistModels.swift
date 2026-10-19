import MonacoCore

@MainActor
final class WatchlistModels {
    static var shared = WatchlistModels(store: UserDefaultsWatchlistStore())

    private let store: any WatchlistStore
    private var models: [String: WatchlistModel] = [:]
    private lazy var unsignedModel = WatchlistModel(userID: "", store: InMemoryWatchlistStore())

    init(store: any WatchlistStore) {
        self.store = store
    }

    func model(for userID: String?) -> WatchlistModel {
        guard let userID, !userID.isEmpty else { return unsignedModel }
        if let model = models[userID] { return model }
        let created = WatchlistModel(userID: userID, store: store)
        models[userID] = created
        return created
    }
}

extension AppEnvironment {
    var watchlist: WatchlistModel {
        WatchlistModels.shared.model(for: viewer?.userID ?? sessionStore.profile?.userID)
    }
}
