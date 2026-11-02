import Foundation
import MonacoAPI

public struct ProposalPriceMove: Equatable, Sendable {
    public let votedQuote: Decimal
    public let currentQuote: Decimal
    public let isSell: Bool

    public init?(votedQuote: Decimal, currentQuote: Decimal, isSell: Bool) {
        guard votedQuote > 0, currentQuote > 0 else { return nil }
        self.votedQuote = votedQuote
        self.currentQuote = currentQuote
        self.isSell = isSell
    }

    public var basisPoints: Int {
        let (from, to) = isSell ? (votedQuote, currentQuote) : (currentQuote, votedQuote)
        let scaled = abs(to - from) * 10_000 / from
        return min(
            TokenAmount.int64(TokenAmount.rounded(scaled, .plain)).map { Int(clamping: $0) } ?? 100_000_000, 100_000_000
        )
    }

    public var percentText: String {
        let tenths = (basisPoints + 5) / 10
        return "\(tenths / 10).\(tenths % 10)%"
    }
}
