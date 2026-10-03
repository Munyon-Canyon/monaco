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
