import Foundation

public struct ProposalPriceMove: Equatable, Sendable {
    public let votedQuote: Int64
    public let currentQuote: Int64
    public let isSell: Bool

    public init?(votedQuote: Int64, currentQuote: Int64, isSell: Bool) {
        guard votedQuote > 0, currentQuote > 0 else { return nil }
        self.votedQuote = votedQuote
        self.currentQuote = currentQuote
        self.isSell = isSell
    }

    public var basisPoints: Int {
        let (from, to) = isSell ? (votedQuote, currentQuote) : (currentQuote, votedQuote)
        let base = UInt64(from)
        let diff = UInt64(abs(to - from))
        let product = diff.multipliedFullWidth(by: 10_000)
        guard product.high < base else { return 100_000_000 }
        let (quotient, remainder) = base.dividingFullWidth(product)
        return min(Int(clamping: remainder * 2 >= base ? quotient + 1 : quotient), 100_000_000)
    }

    public var percentText: String {
        let tenths = (basisPoints + 5) / 10
        return "\(tenths / 10).\(tenths % 10)%"
    }
}
