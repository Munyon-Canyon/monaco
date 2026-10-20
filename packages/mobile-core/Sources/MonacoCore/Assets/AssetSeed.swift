import Foundation
import MonacoAPI

public struct AssetSeed: Hashable, Sendable {
    public let name: String
    public let ticker: String
    public let kind: AssetKind
    public let logoURL: URL?
    public let priceMicros: Int64?
    public let changeBasisPoints: Int64?

    public init(
        name: String, ticker: String, kind: AssetKind = .stock, logoURL: URL? = nil, priceMicros: Int64? = nil,
        changeBasisPoints: Int64? = nil
    ) {
        self.name = name
        self.ticker = ticker
        self.kind = kind
        self.logoURL = logoURL
        self.priceMicros = priceMicros
        self.changeBasisPoints = changeBasisPoints
    }

    public init(_ asset: MarketAsset) {
        self.init(
            name: asset.name, ticker: asset.ticker, kind: asset.kind, logoURL: asset.logoURL,
            priceMicros: asset.priceMicros, changeBasisPoints: asset.changeBasisPoints)
    }

    public init?(_ item: Components.Schemas.FeedItem) {
        guard let symbol = item.symbol, !symbol.isEmpty else { return nil }
        let ticker = AssetSymbolFormatter.display(symbol)
        self.init(
            name: item.assetName ?? ticker, ticker: ticker, priceMicros: item.markMicros,
            changeBasisPoints: item.changeBps.map { Int64($0) })
    }
}
