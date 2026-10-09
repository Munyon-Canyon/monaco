import MonacoAPI

public struct PortfolioSummary: Equatable, Sendable {
    public enum Direction: Equatable, Sendable {
        case up
        case down
        case flat

        init(signedMicros: Int64) {
            self = signedMicros > 0 ? .up : (signedMicros < 0 ? .down : .flat)
        }

        var arrow: String {
            switch self {
            case .up: "▲ "
            case .down: "▼ "
            case .flat: ""
            }
        }
    }

    public struct Row: Equatable, Sendable, Identifiable {
        public let id: String
        public let name: String
        public let pictureURL: String?
        public let valueMicros: Int64
        public let value: String
        public let pnl: String
        public let returnText: String
        public let returnBps: Int64?
        public let slice: String
        public let share: String?
        public let direction: Direction

        init(_ cabal: Components.Schemas.PortfolioCabal, cabalCount: Int) {
            id = cabal.cabal.id
            name = cabal.cabal.name
            pictureURL = cabal.cabal.pictureUrl
            valueMicros = cabal.valueMicros
            value = UsdAmountFormatter.format(micros: cabal.valueMicros)
            pnl = UsdAmountFormatter.format(signedMicros: cabal.pnlMicros)
            returnText = PortfolioSummary.percent(cabal.returnBps)
            returnBps = cabal.returnBps
            slice = PercentFormatter.format(basisPoints: cabal.sliceBps, signed: false)
            share =
                cabalCount > 1
                ? "\(PercentFormatter.format(basisPoints: cabal.sliceBps, signed: false, fractionDigits: 0)) of your cabals"
                : nil
            direction = Direction(signedMicros: cabal.pnlMicros)
        }
    }

    public let totalMicros: Int64
    public let total: String
    public let pnl: String
    public let returnText: String
    public let chip: String
    public let direction: Direction
    public let rows: [Row]
    public let stats: ProfileStats

    public var isEmpty: Bool { rows.isEmpty }

    public init(_ portfolio: Components.Schemas.MyPortfolio) {
        rows = portfolio.cabals.map { Row($0, cabalCount: portfolio.cabals.count) }
        stats = ProfileStats(portfolio)
        totalMicros = portfolio.totalValueMicros
        total = UsdAmountFormatter.format(micros: portfolio.totalValueMicros)
        pnl = UsdAmountFormatter.format(signedMicros: portfolio.pnlMicros)
        returnText = Self.percent(portfolio.returnBps)
        direction = Direction(signedMicros: portfolio.pnlMicros)
        if portfolio.cabals.isEmpty {
            chip = UsdAmountFormatter.format(micros: 0)
        } else {
            let gain = UsdAmountFormatter.format(micros: Int64(clamping: portfolio.pnlMicros.magnitude))
            let percent = Self.percent(portfolio.returnBps.map { Int64(clamping: $0.magnitude) }, signed: false)
            chip = "\(direction.arrow)\(gain) · \(percent)"
        }
    }

    static func percent(_ basisPoints: Int64?, signed: Bool = true) -> String {
        guard let basisPoints else { return "—" }
        return PercentFormatter.format(basisPoints: basisPoints, signed: signed, fractionDigits: 1)
    }
}
