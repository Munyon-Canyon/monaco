import Foundation
import MonacoAPI

public enum ProposalDraft: Equatable, Sendable {
    case buy(symbol: String, usdcMicros: Int64, thesis: String)
    case sell(symbol: String, tokenAmount: Decimal, thesis: String)

    public enum ValidationError: Error, Equatable, Sendable {
        case symbolRequired
        case amountRequired
        case amountExceedsPot
    }

    public func validate(in pot: ProposePot) throws {
        guard !symbol.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw ValidationError.symbolRequired
        }
        guard amount > 0 else { throw ValidationError.amountRequired }
        if case .buy = self, micros > pot.totalMicros { throw ValidationError.amountExceedsPot }
    }

    public var symbol: String {
        switch self {
        case .buy(let symbol, _, _), .sell(let symbol, _, _): symbol
        }
    }

    public var amount: Decimal {
        switch self {
        case .buy(_, let micros, _): TokenAmount.of(micros)
        case .sell(_, let tokens, _): tokens
        }
    }

    public var micros: Int64 {
        if case .buy(_, let micros, _) = self { return micros }
        return 0
    }

    public var thesis: String {
        switch self {
        case .buy(_, _, let thesis), .sell(_, _, let thesis): thesis
        }
    }

    public func with(thesis: String) -> ProposalDraft {
        switch self {
        case .buy(let symbol, let micros, _): .buy(symbol: symbol, usdcMicros: micros, thesis: thesis)
        case .sell(let symbol, let tokens, _): .sell(symbol: symbol, tokenAmount: tokens, thesis: thesis)
        }
    }
}
