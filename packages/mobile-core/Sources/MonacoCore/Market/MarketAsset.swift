import Foundation

public enum MarketIssuer: String, Equatable, Sendable {
    case xstocks
    case tessera
    case prestocks
}

extension MarketIssuer {
    public var displayName: String? {
        switch self {
        case .tessera: "Tessera"
        case .prestocks: "PreStocks"
        case .xstocks: nil
        }
    }
}

public struct MarketListing: Equatable, Sendable, Identifiable {
    public let symbol: String
    public let ticker: String
    public let name: String
    public let issuer: MarketIssuer
    public let kind: AssetKind
    public let isTradable: Bool

    public var id: String { symbol }

    public init(
        symbol: String,
        ticker: String,
        name: String,
        issuer: MarketIssuer,
        kind: AssetKind,
        isTradable: Bool
    ) {
        self.symbol = symbol
        self.ticker = ticker
        self.name = name
        self.issuer = issuer
        self.kind = kind
        self.isTradable = isTradable
    }
}

public struct MarketAssetDetail: Equatable, Sendable {
    public let asset: MarketAsset
    public let otherListings: [MarketListing]

    public init(asset: MarketAsset, otherListings: [MarketListing]) {
        self.asset = asset
        self.otherListings = otherListings
    }
}

public struct MarketAsset: Equatable, Sendable, Identifiable {
    public let symbol: String
    public let ticker: String
    public let name: String
    public let issuer: MarketIssuer
    public let kind: AssetKind
    public let logoURL: URL?
    public let priceMicros: Int64?
    public let priceText: String
    public let changeBasisPoints: Int64?
    public let changeText: String
    public let sparkline: SparklineSeries?
    public let session: MarketSession
    public let status: MarketStatus
    public let showsSessionChip: Bool
    public let isTradable: Bool

    public var id: String { symbol }

    public init(
        symbol: String,
        ticker: String,
        name: String,
        issuer: MarketIssuer,
        kind: AssetKind,
        logoURL: URL?,
        priceMicros: Int64?,
        priceText: String,
        changeBasisPoints: Int64?,
        changeText: String,
        sparkline: SparklineSeries?,
        session: MarketSession,
        status: MarketStatus,
        showsSessionChip: Bool,
        isTradable: Bool
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
        self.isTradable = isTradable
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
