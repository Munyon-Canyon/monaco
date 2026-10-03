import Foundation
import MonacoAPI

public enum MarketMapping {
    public static func asset(_ summary: Components.Schemas.AssetSummary) -> MarketAsset {
        let kind = kind(summary.kind)
        let session = session(summary.session)
        let priceMicros = summary.priceMicros
        let changeBasisPoints = changeBasisPoints(summary.changeBps)
        return MarketAsset(
            symbol: summary.symbol,
            ticker: AssetSymbolFormatter.display(summary.symbol, kind: kind),
            name: name(summary.displayName, symbol: summary.symbol, kind: kind),
            issuer: issuer(summary.issuer),
            kind: kind,
            logoURL: logoURL(summary.logoUrl),
            priceMicros: priceMicros,
            priceText: priceText(priceMicros),
            changeBasisPoints: changeBasisPoints,
            changeText: changeText(changeBasisPoints),
            sparkline: SparklineSeries(usdcMicros: summary.sparklineMicros ?? []),
            session: session,
            status: status(summary.session, session: session),
            showsSessionChip: !summary.session.continuous
        )
    }

    public static func page(_ list: Components.Schemas.AssetList) -> MarketAssetPage {
        MarketAssetPage(assets: list.assets.map(asset), nextCursor: list.nextCursor)
    }

    public static func detail(_ value: Components.Schemas.AssetDetail) -> MarketAssetDetail {
        let kind = kind(value.kind)
        let session = session(value.session)
        let priceMicros = value.priceMicros
        let changeBasisPoints = changeBasisPoints(value.changeBps)
        let asset = MarketAsset(
            symbol: value.symbol,
            ticker: AssetSymbolFormatter.display(value.symbol, kind: kind),
            name: name(value.displayName, symbol: value.symbol, kind: kind),
            issuer: issuer(value.issuer),
            kind: kind,
            logoURL: logoURL(value.logoUrl),
            priceMicros: priceMicros,
            priceText: priceText(priceMicros),
            changeBasisPoints: changeBasisPoints,
            changeText: changeText(changeBasisPoints),
            sparkline: SparklineSeries(usdcMicros: value.sparklineMicros ?? []),
            session: session,
            status: status(value.session, session: session),
            showsSessionChip: !value.session.continuous
        )
        return MarketAssetDetail(asset: asset, otherListings: value.otherListings.map(listing))
    }

    public static func listing(_ value: Components.Schemas.AssetListing) -> MarketListing {
        let kind = kind(value.kind)
        return MarketListing(
            symbol: value.symbol,
            ticker: AssetSymbolFormatter.display(value.symbol, kind: kind),
            name: name(value.displayName, symbol: value.symbol, kind: kind),
            issuer: issuer(value.issuer),
            kind: kind,
            isTradable: value.tradable
        )
    }

    public static func chart(_ value: Components.Schemas.AssetChart) -> AssetChartSeries {
        AssetChartSeries(
            range: chartRange(value.range),
            points: value.points.map {
                AssetChartPointDTO(
                    timestamp: Int64($0.t.timeIntervalSince1970),
                    priceUsdcMicros: $0.closeMicros,
                    openUsdcMicros: $0.openMicros,
                    highUsdcMicros: $0.highMicros,
                    lowUsdcMicros: $0.lowMicros
                )
            }
        )
    }

    public static func session(_ wire: Components.Schemas.MarketSession) -> MarketSession {
        state(wire.state)
    }

    public static func status(
        _ wire: Components.Schemas.MarketSession,
        session: MarketSession
    ) -> MarketStatusDTO {
        let open = session == .open && !wire.continuous
        return MarketStatusDTO(
            session: session,
            isOpen: open,
            afterHours: !wire.continuous && !open,
            nextSession: nextSession(wire.nextState),
            nextTransition: wire.nextTransition,
            holiday: wire.holiday.isEmpty ? nil : wire.holiday,
            earlyClose: wire.earlyClose
        )
    }

    private static func kind(_ wire: Components.Schemas.AssetKind) -> AssetKind {
        switch wire {
        case .equity: .stock
        case .preIpo: .preIpo
        }
    }

    private static func state(_ wire: Components.Schemas.MarketState) -> MarketSession {
        switch wire {
        case .preMarket: .preMarket
        case .open: .open
        case .afterHours: .afterHours
        case .closed: .closed
        }
    }

    private static func chartRange(_ wire: Components.Schemas.AssetChart.RangePayload) -> AssetChartRange {
        switch wire {
        case ._1d: .oneDay
        case ._1w: .oneWeek
        case ._1m: .oneMonth
        case ._3m: .threeMonths
        case ._1y: .oneYear
        case .all: .all
        }
    }

    private static func nextSession(
        _ wire: Components.Schemas.MarketSession.NextStatePayload?
    ) -> MarketSession? {
        switch wire {
        case .preMarket: .preMarket
        case .open: .open
        case .afterHours: .afterHours
        case .closed: .closed
        case ._empty_, nil: nil
        }
    }

    private static func issuer(_ wire: Components.Schemas.AssetIssuer) -> MarketIssuer {
        switch wire {
        case .xstocks: .xstocks
        case .tessera: .tessera
        case .prestocks: .prestocks
        }
    }

    private static func name(_ displayName: String, symbol: String, kind: AssetKind) -> String {
        let formatted = CatalogAssetNameFormatter.format(displayName, kind: kind)
        if !formatted.isEmpty { return formatted }
        return AssetSymbolFormatter.display(symbol, kind: kind)
    }

    private static func logoURL(_ raw: String?) -> URL? {
        guard let raw, let url = URL(string: raw), url.scheme != nil else { return nil }
        return url
    }

    private static func priceText(_ micros: Int64?) -> String {
        guard let micros else { return "—" }
        return UsdAmountFormatter.format(micros: micros)
    }

    private static func changeBasisPoints(_ raw: Int32?) -> Int64? {
        raw.map { Int64($0) }
    }

    private static func changeText(_ basisPoints: Int64?) -> String {
        guard let basisPoints else { return "—" }
        return PercentFormatter.format(basisPoints: basisPoints, signed: true)
    }
}
