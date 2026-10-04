import Foundation
import MonacoAPI

public struct ProposalAsset: Equatable, Sendable {
    public let symbol: String
    public let ticker: String
    public let name: String
    public let logoURL: URL?
    public let kind: AssetKind
    public let decimals: Int
    let tokensPerShareNum: Int64
    let tokensPerShareDen: Int64

    init(_ asset: Components.Schemas.AssetDetail) {
        let kind: AssetKind =
            switch asset.kind {
            case .equity: .stock
            case .preIpo: .preIpo
            }
        symbol = asset.symbol
        self.kind = kind
        ticker = AssetSymbolFormatter.display(asset.symbol, kind: kind)
        name = AssetCatalogDisplayName.format(catalogName: asset.displayName, symbol: asset.symbol, kind: kind)
        logoURL = asset.logoUrl.flatMap(URL.init(string:))
        decimals = min(max(asset.decimals, 0), 18)
        tokensPerShareNum = asset.uiMultiplier.num
        tokensPerShareDen = asset.uiMultiplier.den
    }

    init(unlisted symbol: String) {
        self.symbol = symbol
        kind = .stock
        ticker = AssetSymbolFormatter.display(symbol)
        name = ticker
        logoURL = nil
        decimals = AssetCatalogDefaults.decimals
        tokensPerShareNum = 1
        tokensPerShareDen = 1
    }

    public func amount(of proposal: Proposal) -> String {
        switch proposal.kind {
        case .buy: UsdAmountFormatter.format(micros: proposal.usdcMicros ?? 0)
        case .sell: shares(proposal.tokenAmount ?? 0)
        }
    }

    public func expected(of proposal: Proposal) -> String {
        switch proposal.kind {
        case .sell:
            return "about \(UsdAmountFormatter.format(micros: proposal.quoteOutAmount))"
        case .buy:
            let shares = shares(proposal.quoteOutAmount)
            guard let price = priceMicros(spending: proposal.usdcMicros ?? 0, for: proposal.quoteOutAmount) else {
                return "about \(shares)"
            }
            return "about \(shares) at \(UsdAmountFormatter.format(micros: price))"
        }
    }

    func shares(_ tokenUnits: Int64) -> String {
        TokenQuantityFormatter.label(fromAtomics: String(shareUnits(tokenUnits)), decimals: decimals, kind: kind)
    }

    func priceMicros(spending usdcMicros: Int64, for tokenUnits: Int64) -> Int64? {
        let units = shareUnits(tokenUnits)
        guard usdcMicros > 0, units > 0 else { return nil }
        var scale: UInt64 = 1
        for _ in 0..<decimals { scale *= 10 }
        let product = UInt64(usdcMicros).multipliedFullWidth(by: scale)
        let divisor = UInt64(units)
        guard product.high < divisor else { return nil }
        let quotient = divisor.dividingFullWidth(product).quotient
        return quotient <= UInt64(Int64.max) ? Int64(quotient) : nil
    }

    private func shareUnits(_ tokenUnits: Int64) -> Int64 {
        guard tokensPerShareNum > 0, tokensPerShareDen > 0, tokensPerShareNum != tokensPerShareDen else {
            return tokenUnits
        }
        let (scaled, overflow) = tokenUnits.multipliedReportingOverflow(by: tokensPerShareDen)
        return overflow ? tokenUnits / tokensPerShareNum * tokensPerShareDen : scaled / tokensPerShareNum
    }
}
