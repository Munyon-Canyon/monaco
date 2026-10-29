import Foundation
import MonacoAPI

public enum PriceSource: String, Equatable, Sendable {
    case jupiter
    case coingecko
}

public struct MarketStatsFacts: Equatable, Sendable {
    public let listingNetworks: [Components.Schemas.Network]
    public let tradesContinuously: Bool
    public let priceSource: PriceSource?
    public let priceAsOf: Date?
    public let pollInterval: Duration

    public init(
        listingNetworks: [Components.Schemas.Network], tradesContinuously: Bool, priceSource: PriceSource?,
        priceAsOf: Date?, pollInterval: Duration
    ) {
        self.listingNetworks = listingNetworks
        self.tradesContinuously = tradesContinuously
        self.priceSource = priceSource
        self.priceAsOf = priceAsOf
        self.pollInterval = pollInterval
    }

    public init(_ asset: Components.Schemas.AssetDetail) {
        self.init(
            listingNetworks: [.solana], tradesContinuously: asset.session.continuous,
            priceSource: asset.priceSource.flatMap { PriceSource(rawValue: $0.rawValue) },
            priceAsOf: asset.priceAsOf, pollInterval: .seconds(asset.pricePollIntervalSeconds))
    }
}
