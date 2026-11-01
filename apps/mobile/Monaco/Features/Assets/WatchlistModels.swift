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
        adoptQueuedStars(into: created)
        models[userID] = created
        return created
    }

    private func adoptQueuedStars(into model: WatchlistModel) {
        for symbol in unsignedModel.symbols {
            if !model.isWatched(symbol) { model.toggle(symbol) }
            unsignedModel.remove(symbol)
        }
    }
}

extension AppEnvironment {
    var watchlist: WatchlistModel {
        WatchlistModels.shared.model(for: viewer?.userID ?? sessionStore.profile?.userID)
    }
}
