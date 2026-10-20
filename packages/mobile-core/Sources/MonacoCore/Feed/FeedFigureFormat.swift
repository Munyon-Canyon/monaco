import Foundation
import MonacoAPI

public enum FeedSide: Equatable, Sendable {
    case buy
    case sell
}

public enum FeedAuthorReturn: Equatable, Sendable {
    case percent(String)
    case hidden
}

public enum FeedFigureFormat {
    public static func side(_ item: Components.Schemas.FeedItem) -> FeedSide {
        item.action == "sell" ? .sell : .buy
    }

    public static func authorReturn(bps: Int64?, isPrivate: Bool) -> FeedAuthorReturn? {
        if isPrivate { return .hidden }
        return bps.map { .percent(PercentFormatter.format(basisPoints: $0, signed: true, fractionDigits: 1)) }
    }

    public static func authorReturn(_ item: Components.Schemas.FeedItem) -> FeedAuthorReturn? {
        authorReturn(bps: item.return1mBps, isPrivate: item.returnPrivate)
    }

    public static func authorReturn(_ author: Components.Schemas.CommentAuthor) -> FeedAuthorReturn? {
        authorReturn(bps: author.return1mBps, isPrivate: author.returnPrivate)
    }

    public static func amount(_ item: Components.Schemas.FeedItem) -> String? {
        item.usdcMicros.map { UsdAmountFormatter.format(micros: $0) }
    }

    public static func change(_ item: Components.Schemas.FeedItem) -> String? {
        item.changeBps.map { PercentFormatter.format(basisPoints: $0, signed: true) }
    }

    public static func detail(_ item: Components.Schemas.FeedItem) -> String? {
        switch item.kind {
        case FeedKind.trade.rawValue: tradeDetail(item)
        case FeedKind.proposal.rawValue, FeedKind.threshold.rawValue: potShare(item)
        default: nil
        }
    }

    static func tradeDetail(_ item: Components.Schemas.FeedItem) -> String? {
        let verb = side(item) == .sell ? "Sold" : "Bought"
        let units = units(item).map { "\(verb) \($0)" }
        let average = item.priceMicros.map { "Avg \(UsdAmountFormatter.format(micros: $0))" }
        let parts = [units, average].compactMap { $0 }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    static func potShare(_ item: Components.Schemas.FeedItem) -> String? {
        guard let bps = item.shareOfPotBps else { return nil }
        let share = PercentFormatter.format(basisPoints: bps, signed: false)
        guard let cabal = item.cabalName, !cabal.isEmpty else { return "\(share) of the pot" }
        return "\(share) of \(cabal)’s pot"
    }

    static func units(_ item: Components.Schemas.FeedItem) -> String? {
        guard let amount = item.tokenAmount, let decimals = item.tokenDecimals, amount > 0,
            let quantity = TokenQuantityFormatter.quantity(fromAtomics: String(amount), decimals: decimals)
        else { return nil }
        return TokenQuantityFormatter.label(quantity: quantity, kind: .stock)
            .replacingOccurrences(of: "share", with: "unit")
    }
}
