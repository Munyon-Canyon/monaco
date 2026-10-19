import Foundation
import MonacoCore

@MainActor
final class UserDefaultsWatchlistStore: WatchlistStore {
    private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    func symbols(for userID: String) -> [String] {
        WatchlistRecord.decode(defaults.data(forKey: WatchlistRecord.key(for: userID)))
    }

    func save(_ symbols: [String], for userID: String) {
        defaults.set(WatchlistRecord.encode(symbols), forKey: WatchlistRecord.key(for: userID))
    }
}
