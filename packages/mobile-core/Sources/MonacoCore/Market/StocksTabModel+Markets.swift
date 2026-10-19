import Foundation

extension StocksTabModel {
    public nonisolated static let moverLimit = 4
    public nonisolated static let moverMinimum = 2
    public nonisolated static let popularNowLimit = 3

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

    public var topMovers: [MarketAsset] {
        let ranked = popular.rows.enumerated().compactMap { offset, asset in
            asset.changeBasisPoints.map { (offset: offset, asset: asset, size: abs($0)) }
        }
        guard ranked.count >= Self.moverMinimum else { return [] }
        let ordered = ranked.sorted { $0.size == $1.size ? $0.offset < $1.offset : $0.size > $1.size }
        return ordered.prefix(Self.moverLimit).map(\.asset)
    }

    public var popularNow: [MarketAsset] {
        Array(popular.rows.prefix(Self.popularNowLimit))
    }

    public var staleMinutes: Int? {
        guard isStale, let lastLoadedAt else { return nil }
        return MarketStaleness.minutes(from: lastLoadedAt, to: now())
    }
}
