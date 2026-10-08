import Foundation
import MonacoAPI
import MonacoCore

/// A stock the propose flow can show: catalog search rows carry no price, popular rows do.
struct ProposeStock: Hashable, Identifiable {
    let symbol: String
    let name: String
    var priceMicros: Int64?
    var change24h: String?
    var isTradable = true
    var assetKind: AssetKind = .stock
    var tokenDecimals: Int?

    var id: String { symbol }

    /// Ticker without the xStock suffix, e.g. "AAPL".
    var ticker: String { AssetSymbolFormatter.display(symbol, kind: assetKind) }

    var priceUsd: Decimal? {
        priceMicros.map { Decimal($0) / Decimal(1_000_000) }
    }

    init(
        symbol: String,
        name: String,
        priceMicros: Int64? = nil,
        change24h: String? = nil,
        isTradable: Bool = true,
        assetKind: AssetKind = .stock,
        tokenDecimals: Int? = nil
    ) {
        self.symbol = symbol
        self.name = name
        self.priceMicros = priceMicros
        self.change24h = change24h
        self.isTradable = isTradable
        self.assetKind = assetKind
        self.tokenDecimals = tokenDecimals
    }

    static func displayName(symbol: String, catalogName: String? = nil, kind: AssetKind = .stock) -> String {
        AssetCatalogDisplayName.format(catalogName: catalogName ?? "", symbol: symbol, kind: kind)
    }

    init(symbol: String, kind: AssetKind = .stock, tokenDecimals: Int? = nil) {
        self.init(
            symbol: symbol,
            name: Self.displayName(symbol: symbol, kind: kind),
            assetKind: kind,
            tokenDecimals: tokenDecimals
        )
    }
}

/// The pot numbers the propose screens need, read once from the cabal view.
struct ProposePot: Equatable {
    let groupId: String
    let name: String
    /// Buy ceiling: the whole pot (cash plus holdings), matching the backend rule.
    let totalMicros: Int64
    let holdings: [PotRowDTO]

    init(view: GroupViewDTO) {
        groupId = view.id
        name = view.name
        totalMicros = ProposeMath.micros(fromUsd: view.resolvedPotTotalUsd) ?? 0
        holdings = view.pot.filter { row in
            row.symbol.uppercased() != "USDC" && (Int64(row.tokenAmount ?? "0") ?? 0) > 0
        }
    }
}

/// Fixed-point conversions for the propose flows. USDC has 6 decimals; xStock tokens have 8.
enum ProposeMath {
    static let usdcScale = Decimal(1_000_000)

    static func micros(fromUsd raw: String) -> Int64? {
        guard
            let value = Decimal(
                string: raw.trimmingCharacters(in: .whitespaces), locale: Locale(identifier: "en_US_POSIX"))
        else {
            return nil
        }
        return micros(fromUsd: value)
    }

    static func micros(fromUsd value: Decimal) -> Int64? {
        guard value >= 0 else { return nil }
        return rounded(value * usdcScale, mode: .plain)
    }

    static func usd(fromMicros micros: Int64) -> Decimal {
        Decimal(micros) / usdcScale
    }

    static func shares(
        fromAtomics raw: String, decimals: Int = ProposalShareFormatter.defaultDecimals, multiplier: Decimal = 1
    ) -> Decimal? {
        guard let qty = TokenQuantityFormatter.quantity(fromAtomics: raw, decimals: decimals), multiplier > 0 else {
            return nil
        }
        return qty * multiplier
    }

    private static func rounded(_ value: Decimal, mode: NSDecimalNumber.RoundingMode) -> Int64? {
        var source = value
        var result = Decimal()
        NSDecimalRound(&result, &source, 0, mode)
        return (result as NSDecimalNumber).int64Value
    }
}
