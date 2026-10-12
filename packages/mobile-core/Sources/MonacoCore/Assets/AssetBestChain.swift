import Foundation
import MonacoAPI

public struct AssetBestChain: Equatable, Sendable {
    public let symbol: String
    public let network: Components.Schemas.Network
    public let probeMicros: Int64
    public let netShareNanos: Int64
    public let quotedAt: Date
    public let comparison: Components.Schemas.PriceComparison

    public init?(_ wire: Components.Schemas.AssetBestChain) {
        guard let probe = Int64(wire.amountMicros), probe > 0, let shares = Int64(wire.netShareNanos) else {
            return nil
        }
        symbol = wire.symbol
        network = wire.network
        probeMicros = probe
        netShareNanos = shares
        quotedAt = wire.quotedAt
        comparison = wire.priceComparison
    }

    public var chainName: String { BestPrice.chainName(network) }

    public var headline: String { "Best price on \(chainName)" }

    public func shareNanos(forMicros amount: Int64) -> Int64 {
        Self.scale(netShareNanos, amount: amount, probe: probeMicros)
    }

    public func bestPrice(forMicros amount: Int64) -> BestPrice? {
        guard amount > 0 else { return nil }
        let quotes = comparison.quotes.compactMap { quote -> Components.Schemas.RouteQuote? in
            guard let shares = Int64(quote.netShareNanos), let micros = Int64(quote.netMicros) else { return nil }
            var scaled = quote
            scaled.netShareNanos = String(Self.scale(shares, amount: amount, probe: probeMicros))
            scaled.netMicros = String(Self.scale(micros, amount: amount, probe: probeMicros))
            return scaled
        }
        var scaled = comparison
        scaled.quotes = quotes
        return BestPrice(scaled, isSell: false)
    }

    static func scale(_ value: Int64, amount: Int64, probe: Int64) -> Int64 {
        guard value > 0, amount > 0, probe > 0 else { return 0 }
        let (high, low) = value.multipliedFullWidth(by: amount)
        guard high < probe else { return .max }
        return probe.dividingFullWidth((high, low)).quotient
    }
}
