import MonacoAPI
import Observation

@Observable
@MainActor
public final class WatchlistQuotes {
    public nonisolated static let previewCount = 3

    public enum Row: Equatable, Sendable, Identifiable {
        case quoted(MarketAsset)
        case loading(symbol: String)
        case unavailable(symbol: String)

        public var id: String {
            switch self {
            case .quoted(let asset): asset.symbol
            case .loading(let symbol), .unavailable(let symbol): symbol
            }
        }
    }

    private enum Read: Sendable {
        case quote(MarketAsset)
        case gone
        case failed
        case cancelled
    }

    private var quotes: [String: MarketAsset] = [:]
    private var failed: Set<String> = []
    @ObservationIgnored private let api: APIClient
    @ObservationIgnored private let watchlist: WatchlistModel

    public init(api: APIClient, watchlist: WatchlistModel) {
        self.api = api
        self.watchlist = watchlist
    }

    public var rows: [Row] {
        watchlist.symbols.prefix(Self.previewCount).map { symbol in
            if let asset = quotes[symbol] { return .quoted(asset) }
            return failed.contains(symbol) ? .unavailable(symbol: symbol) : .loading(symbol: symbol)
        }
    }

    public func load() async {
        let symbols = Array(watchlist.symbols.prefix(Self.previewCount))
        let api = api
        let reads = await withTaskGroup(of: (String, Read).self) { group in
            for symbol in symbols { group.addTask { (symbol, await Self.read(api, symbol)) } }
            var results: [String: Read] = [:]
            for await (symbol, read) in group { results[symbol] = read }
            return results
        }
        for (symbol, read) in reads { apply(read, to: symbol) }
        quotes = quotes.filter { watchlist.isWatched($0.key) }
        failed = failed.filter { watchlist.isWatched($0) }
    }

    private func apply(_ read: Read, to symbol: String) {
        switch read {
        case .quote(let asset):
            quotes[symbol] = asset
            failed.remove(symbol)
        case .gone:
            watchlist.remove(symbol)
        case .failed:
            if quotes[symbol] == nil { failed.insert(symbol) }
        case .cancelled:
            break
        }
    }

    private nonisolated static func read(_ api: APIClient, _ symbol: String) async -> Read {
        do {
            let detail = try await api.read { try await $0.getAsset(path: .init(symbol: symbol)).ok.body.json }
            return .quote(MarketMapping.detail(detail).asset)
        } catch {
            switch APIError(error) {
            case .cancelled: return .cancelled
            case .problem(let problem) where problem.code == .known(.assetNotFound): return .gone
            default: return .failed
            }
        }
    }
}
