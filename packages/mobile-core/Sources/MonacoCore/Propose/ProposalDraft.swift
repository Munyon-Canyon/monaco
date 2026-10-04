import Foundation

public enum ProposalDraft: Equatable, Sendable {
    case buy(symbol: String, usdcMicros: Int64, thesis: String)
    case sell(symbol: String, tokenAmount: Int64, thesis: String)

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
        if case .buy = self, amount > pot.totalMicros { throw ValidationError.amountExceedsPot }
    }

    public var symbol: String {
        switch self {
        case .buy(let symbol, _, _), .sell(let symbol, _, _): symbol
        }
    }

    public var amount: Int64 {
        switch self {
        case .buy(_, let amount, _): amount
        case .sell(_, let amount, _): amount
        }
    }
}
