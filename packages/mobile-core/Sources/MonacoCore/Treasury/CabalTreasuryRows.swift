import Foundation
import MonacoAPI

public struct CabalTreasuryRows: Equatable, Sendable {
    public struct Cash: Equatable, Sendable {
        public let label: String
        public let apy: String
        public let amount: String
    }

    public enum Trailing: Equatable, Sendable {
        case change(text: String, direction: PortfolioSummary.Direction)
        case estimate(String)
        case none
    }

    public struct Position: Equatable, Sendable, Identifiable {
        public let symbol: String
        public let ticker: String
        public let name: String
        public let detail: String
        public let value: String
        public let trailing: Trailing
        public var logoURL: URL?

        public var id: String { symbol }
    }

    public static let atEstimate = "at estimate"
    public static let positionsLabel = "Positions"
    public static let botLabel = "Trading bot"
    public static let cashOutLabel = "Cash out my share"
    public static let historyLabel = "Treasury history"

    public let cash: Cash
    public private(set) var positions: [Position]
    public let cashOut: String?

    public init(_ pot: Components.Schemas.CabalPot) {
        cash = Cash(
            label: MoneyCopy.cashLabel, apy: CabalYield.apy, amount: UsdAmountFormatter.format(micros: pot.cashMicros))
        positions = pot.holdings
            .sorted { ($0.valueMicros, $1.symbol) > ($1.valueMicros, $0.symbol) }
            .map(Self.position)
        cashOut = (pot.me?.valueMicros ?? 0) > 0 ? CabalHeaderCopy.compactDollars(pot.me?.valueMicros ?? 0) : nil
    }

    public static func history(_ move: CabalHeaderCopy.Move?) -> String? {
        move?.line
    }

    public func withLogos(_ logos: [String: URL]) -> CabalTreasuryRows {
        var copy = self
        copy.positions = positions.map { position in
            var position = position
            position.logoURL = logos[position.symbol]
            return position
        }
        return copy
    }

    private static func position(_ holding: Components.Schemas.CabalHolding) -> Position {
        let units = Units(holding.units)
        let isPreIpo = AssetKind(raw: holding.kind.rawValue) == .preIpo
        let shares = units.map { "\($0.display) sh" } ?? "\(holding.units) sh"
        let detail: String
        if isPreIpo {
            detail = "\(PreIpoCopy.chipLabel) \u{00B7} \(shares)"
        } else if let average = units.flatMap({ $0.average(costMicros: holding.costBasisMicros) }) {
            detail = "\(shares) \u{00B7} avg \(UsdAmountFormatter.format(micros: average))"
        } else {
            detail = shares
        }
        return Position(
            symbol: holding.symbol, ticker: AssetSymbolFormatter.display(holding.symbol), name: holding.displayName,
            detail: detail, value: UsdAmountFormatter.format(micros: holding.valueMicros),
            trailing: isPreIpo ? .estimate(atEstimate) : change(holding))
    }

    private static func change(_ holding: Components.Schemas.CabalHolding) -> Trailing {
        guard holding.costBasisMicros > 0,
            let basisPoints = mulDiv(holding.pnlMicros, 10_000, holding.costBasisMicros)
        else { return .none }
        return .change(
            text: PercentFormatter.format(basisPoints: basisPoints, signed: true, fractionDigits: 1),
            direction: PortfolioSummary.Direction(signedMicros: basisPoints))
    }

    struct Units: Equatable {
        let scaled: Int64
        let scale: Int

        init?(_ text: String) {
            let parts = text.split(separator: ".", maxSplits: 1, omittingEmptySubsequences: false)
            guard let whole = parts.first, !whole.isEmpty else { return nil }
            var fraction = parts.count > 1 ? Substring(parts[1]) : ""
            while fraction.last == "0" { fraction = fraction.dropLast() }
            guard fraction.count <= 12, let scaled = Int64(String(whole) + fraction), scaled > 0,
                whole.allSatisfy(\.isNumber), fraction.allSatisfy(\.isNumber)
            else { return nil }
            self.scaled = scaled
            scale = fraction.count
        }

        var display: String {
            let hundredths: Int64
            if scale > 2 {
                guard let divisor = Self.power(scale - 2) else { return "0" }
                hundredths = scaled / divisor + (scaled % divisor * 2 >= divisor ? 1 : 0)
            } else {
                hundredths = scaled * (Self.power(2 - scale) ?? 1)
            }
            guard hundredths > 0 else { return "<0.01" }
            let whole = hundredths / 100
            let fraction = hundredths % 100
            if fraction == 0 { return String(whole) }
            if fraction % 10 == 0 { return "\(whole).\(fraction / 10)" }
            let padding: String = fraction < 10 ? "0" : ""
            return "\(whole).\(padding)\(fraction)"
        }

        func average(costMicros: Int64) -> Int64? {
            guard costMicros > 0, let factor = Self.power(scale) else { return nil }
            return mulDiv(costMicros, factor, scaled)
        }

        private static func power(_ exponent: Int) -> Int64? {
            var result: Int64 = 1
            for _ in 0..<exponent {
                let (next, overflow) = result.multipliedReportingOverflow(by: 10)
                if overflow { return nil }
                result = next
            }
            return result
        }
    }

    static func mulDiv(_ value: Int64, _ factor: Int64, _ divisor: Int64) -> Int64? {
        let (product, overflow) = value.multipliedReportingOverflow(by: factor)
        guard divisor != 0, !overflow else { return nil }
        let quotient = product / divisor
        let remainder = product % divisor
        guard remainder.magnitude * 2 >= divisor.magnitude else { return quotient }
        return (product < 0) != (divisor < 0) ? quotient - 1 : quotient + 1
    }
}
