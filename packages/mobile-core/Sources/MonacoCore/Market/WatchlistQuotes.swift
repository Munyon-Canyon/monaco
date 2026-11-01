import MonacoAPI
import Observation

@Observable
@MainActor
public final class WatchlistQuotes {
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
        watchlist.symbols.map { symbol in
            if let asset = quotes[symbol] { return .quoted(asset) }
            return failed.contains(symbol) ? .unavailable(symbol: symbol) : .loading(symbol: symbol)
        }
    }

    public func load() async { await fetch(watchlist.symbols) }

    public func loadMissing() async {
        await fetch(watchlist.symbols.filter { quotes[$0] == nil })
    }

    private func fetch(_ symbols: [String]) async {
        guard !symbols.isEmpty else { return }
        let reads = await Self.readBatch(api, symbols)
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

    private nonisolated static func readBatch(_ api: APIClient, _ symbols: [String]) async -> [String: Read] {
        let joined = symbols.joined(separator: ",")
        do {
            let list = try await api.read { try await $0.getAssetQuotes(query: .init(symbols: joined)).ok.body.json }
            let now = WallClock.now()
            var quoted: [String: MarketAsset] = [:]
            for summary in list.assets { quoted[summary.symbol] = MarketMapping.asset(summary, now: now) }
            var results: [String: Read] = [:]
            for symbol in symbols {
                if let asset = quoted[symbol] { results[symbol] = .quote(asset) } else { results[symbol] = .gone }
            }
            return results
        } catch {
            switch APIError(error) {
            case .problem(let problem) where problem.status == 404:
                return await readEach(api, symbols)
            case .cancelled:
                return Self.every(symbols, .cancelled)
            default:
                return Self.every(symbols, .failed)
            }
        }
    }

    private nonisolated static func every(_ symbols: [String], _ read: Read) -> [String: Read] {
        var results: [String: Read] = [:]
        for symbol in symbols { results[symbol] = read }
        return results
    }

    private nonisolated static func readEach(_ api: APIClient, _ symbols: [String]) async -> [String: Read] {
        await withTaskGroup(of: (String, Read).self) { group in
            for symbol in symbols { group.addTask { (symbol, await readOne(api, symbol)) } }
            var results: [String: Read] = [:]
            for await (symbol, read) in group { results[symbol] = read }
            return results
        }
    }

    private nonisolated static func readOne(_ api: APIClient, _ symbol: String) async -> Read {
        do {
            let detail = try await api.read { try await $0.getAsset(path: .init(symbol: symbol)).ok.body.json }
            return .quote(MarketMapping.detail(detail, now: WallClock.now()).asset)
        } catch {
            switch APIError(error) {
            case .cancelled: return .cancelled
            case .problem(let problem) where problem.code == .known(.assetNotFound): return .gone
            default: return .failed
            }
        }
    }
}
