import Foundation
import MonacoAPI

public struct CabalPotSummary: Equatable, Sendable {
    public enum PotState: Equatable, Sendable {
        case zero
        case cashOnly
        case invested
    }

    public enum Slice: Equatable, Sendable {
        case none
        case stake(value: String, ofPot: String, gain: String)
    }

    public struct Row: Equatable, Sendable, Identifiable {
        public let symbol: String
        public let ticker: String
        public let name: String
        public let detail: String
        public let value: String
        public let gain: String
        public var logoURL: URL?

        public var id: String { symbol }
    }

    public enum Swatch: Equatable, Sendable {
        case stock(step: Int)
        case cash

        public static let steps = 5

        public static func forStock(at index: Int) -> Swatch {
            .stock(step: min(max(index, 0), steps - 1))
        }
    }

    public struct Segment: Equatable, Sendable, Identifiable {
        public let label: String
        public let percent: String
        public let basisPoints: Int
        public let isCash: Bool
        public let swatch: Swatch

        public var id: String { label }
    }

    public let potValue: String
    public let allTime: String
    public let cash: String
    public let invested: String
    public let slice: Slice?
    public private(set) var holdings: [Row]
    public let sellable: [ProposeHolding]
    public let legend: [Segment]
    public let state: PotState

    public init(_ pot: Components.Schemas.CabalPot) {
        potValue = UsdAmountFormatter.format(micros: pot.potValueMicros)
        allTime = UsdAmountFormatter.format(signedMicros: pot.pnlMicros)
        cash = UsdAmountFormatter.format(micros: pot.cashMicros)
        invested = UsdAmountFormatter.format(micros: pot.holdings.reduce(0) { $0 + $1.valueMicros })
        slice = pot.me.map { me in
            guard me.shareUnits != 0 else { return .none }
            return .stake(
                value: UsdAmountFormatter.format(micros: me.valueMicros),
                ofPot: "\(Self.percent(basisPoints: Int(me.sliceBps))) of the pot",
                gain: UsdAmountFormatter.format(signedMicros: me.pnlMicros))
        }
        holdings = pot.holdings.map { holding in
            Row(
                symbol: holding.symbol,
                ticker: AssetSymbolFormatter.display(holding.symbol),
                name: holding.displayName,
                detail: "\(Self.shares(holding.units)) · \(UsdAmountFormatter.format(micros: holding.priceMicros))",
                value: UsdAmountFormatter.format(micros: holding.valueMicros),
                gain: UsdAmountFormatter.format(signedMicros: holding.pnlMicros))
        }
        sellable = pot.holdings.map(ProposeHolding.init)
        legend =
            pot.holdings.enumerated().map { index, holding in
                Segment(
                    label: AssetSymbolFormatter.display(holding.symbol),
                    percent: Self.percent(basisPoints: Int(holding.weightBps)),
                    basisPoints: Int(holding.weightBps), isCash: false, swatch: .forStock(at: index))
            } + [
                Segment(
                    label: "Cash", percent: Self.percent(basisPoints: Int(pot.cashWeightBps)),
                    basisPoints: Int(pot.cashWeightBps), isCash: true, swatch: .cash)
            ]
        state =
            if pot.potValueMicros == 0 { .zero } else if pot.holdings.isEmpty { .cashOnly } else { .invested }
    }

    public func withLogos(_ logos: [String: URL]) -> CabalPotSummary {
        var copy = self
        copy.holdings = holdings.map { row in
            var row = row
            row.logoURL = logos[row.symbol]
            return row
        }
        return copy
    }

    static func percent(basisPoints: Int) -> String {
        if basisPoints > 0, basisPoints < 100 { return "<1%" }
        return "\((basisPoints + 50) / 100)%"
    }

    static func shares(_ units: String) -> String {
        var trimmed = Substring(units)
        if trimmed.contains(".") {
            while trimmed.last == "0" { trimmed = trimmed.dropLast() }
            if trimmed.last == "." { trimmed = trimmed.dropLast() }
        }
        return trimmed == "1" ? "1 share" : "\(trimmed) shares"
    }
}
