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

        public var id: String { symbol }
    }

    public struct Segment: Equatable, Sendable, Identifiable {
        public let label: String
        public let percent: String
        public let basisPoints: Int
        public let isCash: Bool

        public var id: String { label }
    }

    public let potValue: String
    public let allTime: String
    public let cash: String
    public let slice: Slice?
    public let holdings: [Row]
    public let sellable: [ProposeHolding]
    public let legend: [Segment]
    public let state: PotState

    public init(_ pot: Components.Schemas.CabalPot) {
        potValue = UsdAmountFormatter.format(micros: pot.potValueMicros)
        allTime = UsdAmountFormatter.format(signedMicros: pot.pnlMicros)
        cash = UsdAmountFormatter.format(micros: pot.cashMicros)
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
            pot.holdings.map { holding in
                Segment(
                    label: AssetSymbolFormatter.display(holding.symbol),
                    percent: Self.percent(basisPoints: Int(holding.weightBps)),
                    basisPoints: Int(holding.weightBps), isCash: false)
            } + [
                Segment(
                    label: "Cash", percent: Self.percent(basisPoints: Int(pot.cashWeightBps)),
                    basisPoints: Int(pot.cashWeightBps), isCash: true)
            ]
        state =
            if pot.potValueMicros == 0 { .zero } else if pot.holdings.isEmpty { .cashOnly } else { .invested }
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
