import Foundation

public struct MarketAsset: Equatable, Sendable, Identifiable {
    public let symbol: String
    public let ticker: String
    public let name: String
    public let issuer: String
    public let kind: AssetKind
    public let logoURL: URL?
    public let priceMicros: Int64?
    public let priceText: String
    public let changeBasisPoints: Int64?
    public let changeText: String
    public let sparkline: SparklineSeries?
    public let session: MarketSession
    public let status: MarketStatusDTO
    public let showsSessionChip: Bool

    public var id: String { symbol }

    public init(
        symbol: String,
        ticker: String,
        name: String,
        issuer: String,
        kind: AssetKind,
        logoURL: URL?,
        priceMicros: Int64?,
        priceText: String,
        changeBasisPoints: Int64?,
        changeText: String,
        sparkline: SparklineSeries?,
        session: MarketSession,
        status: MarketStatusDTO,
        showsSessionChip: Bool
    ) {
        self.symbol = symbol
        self.ticker = ticker
        self.name = name
        self.issuer = issuer
        self.kind = kind
        self.logoURL = logoURL
        self.priceMicros = priceMicros
        self.priceText = priceText
        self.changeBasisPoints = changeBasisPoints
        self.changeText = changeText
        self.sparkline = sparkline
        self.session = session
        self.status = status
        self.showsSessionChip = showsSessionChip
    }
}

public struct MarketAssetPage: Equatable, Sendable {
    public let assets: [MarketAsset]
    public let nextCursor: String?

    public init(assets: [MarketAsset], nextCursor: String?) {
        self.assets = assets
        self.nextCursor = nextCursor
    }
}
