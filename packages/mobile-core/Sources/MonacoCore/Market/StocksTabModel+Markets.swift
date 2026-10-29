import Foundation

extension StocksTabModel {
    public nonisolated static let sectionLimit = 5

    public enum Sort: CaseIterable, Hashable, Sendable {
        case standard, volume, tradeCount, name, priceChange

        public var title: String {
            switch self {
            case .standard: "Default"
            case .volume: "Volume"
            case .tradeCount: "Trade count"
            case .name: "Name A–Z"
            case .priceChange: "Price change"
            }
        }

        public var isAvailable: Bool { self == .standard }

        public nonisolated static let unavailableNote = "More sorts open soon"
    }

    public var watchlistCount: Int { watchlist?.symbols.count ?? 0 }

    public var watchlistRows: [WatchlistQuotes.Row] { quotes?.rows ?? [] }

    public var watchlistPreview: [WatchlistQuotes.Row] {
        Array(watchlistRows.filter { !Self.isPreIpo($0) }.prefix(Self.sectionLimit))
    }

    public var preIpoPreview: [MarketAsset] { Array(preIpo.rows.prefix(Self.sectionLimit)) }

    public func subline(for asset: MarketAsset) -> String {
        MarketFacts.subline(facts[asset.symbol], ticker: asset.ticker)
    }

    private static func isPreIpo(_ row: WatchlistQuotes.Row) -> Bool {
        if case .quoted(let asset) = row { return asset.kind == .preIpo }
        return false
    }

    public var staleMinutes: Int? {
        guard isStale, let lastLoadedAt else { return nil }
        return MarketStaleness.minutes(from: lastLoadedAt, to: now())
    }

    public func watchlistChanged() async {
        async let watched: Void = loadQuotes()
        async let known: Void = loadFacts()
        _ = await (watched, known)
    }

    func loadFacts() async {
        let symbols = Array((watchlist?.symbols ?? []).prefix(WatchlistModel.capacity))
        guard !symbols.isEmpty else {
            facts = [:]
            return
        }
        let joined = symbols.joined(separator: ",")
        do {
            let wire = try await api.read { client in
                try await client.getMyMarketFacts(query: .init(symbols: joined)).ok.body.json
            }
            facts = MarketFacts.map(wire).filter { symbols.contains($0.key) }
        } catch {
            return
        }
    }
}
