import Foundation
import Observation

@MainActor
public protocol WatchlistStore {
    func symbols(for userID: String) -> [String]
    func save(_ symbols: [String], for userID: String)
}

@MainActor
public final class InMemoryWatchlistStore: WatchlistStore {
    private var saved: [String: [String]]

    public init(_ saved: [String: [String]] = [:]) {
        self.saved = saved
    }

    public func symbols(for userID: String) -> [String] { saved[userID] ?? [] }

    public func save(_ symbols: [String], for userID: String) { saved[userID] = symbols }
}

@Observable
@MainActor
public final class WatchlistModel {
    public nonisolated static let capacity = 50

    public enum Change: Equatable, Sendable {
        case added, removed, full
    }

    public private(set) var symbols: [String]
    @ObservationIgnored private let userID: String
    @ObservationIgnored private let store: any WatchlistStore

    public init(userID: String, store: any WatchlistStore) {
        self.userID = userID
        self.store = store
        symbols = Self.clean(store.symbols(for: userID))
    }

    public func isWatched(_ symbol: String) -> Bool { symbols.contains(symbol) }

    @discardableResult
    public func toggle(_ symbol: String) -> Change {
        if isWatched(symbol) {
            remove(symbol)
            return .removed
        }
        guard symbols.count < Self.capacity else { return .full }
        symbols.append(symbol)
        store.save(symbols, for: userID)
        return .added
    }

    public func remove(_ symbol: String) {
        guard isWatched(symbol) else { return }
        symbols.removeAll { $0 == symbol }
        store.save(symbols, for: userID)
    }

    private static func clean(_ stored: [String]) -> [String] {
        var seen = Set<String>()
        let unique = stored.filter { !$0.isEmpty && seen.insert($0).inserted }
        return Array(unique.prefix(capacity))
    }
}

public enum WatchlistRecord {
    public static func key(for userID: String) -> String { "markets-watchlist.\(userID)" }

    public static func encode(_ symbols: [String]) -> Data {
        (try? JSONEncoder().encode(symbols)) ?? Data("[]".utf8)
    }

    public static func decode(_ data: Data?) -> [String] {
        guard let data else { return [] }
        return (try? JSONDecoder().decode([String].self, from: data)) ?? []
    }
}

public enum WatchlistCopy {
    public static let fullMessage = "Your watchlist holds up to \(WatchlistModel.capacity) stocks."

    public static func countLine(_ count: Int) -> String {
        switch count {
        case 0: "No stocks selected"
        case 1: "1 stock selected"
        default: "\(count) stocks selected"
        }
    }
}
