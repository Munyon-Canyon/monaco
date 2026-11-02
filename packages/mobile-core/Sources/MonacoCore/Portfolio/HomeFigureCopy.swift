import Foundation

public enum HomeFigureCopy {
    public struct Figure: Equatable, Sendable {
        public let whole: String
        public let cents: String
    }

    public struct Line: Equatable, Sendable {
        public let amount: String
        public let percent: String
        public let words: String
        public let direction: PortfolioSummary.Direction
    }

    public static func netWorth(micros: Int64) -> Figure {
        let text = UsdAmountFormatter.format(micros: UsdAmountFormatter.flooredToCents(micros))
        return Figure(whole: String(text.dropLast(3)), cents: String(text.suffix(3)))
    }

    public static func line(pnlMicros: Int64, returnBps: Int64, range: LeaderboardRange) -> Line {
        Line(
            amount: UsdAmountFormatter.format(signedMicros: pnlMicros),
            percent: PercentFormatter.format(basisPoints: abs(returnBps), signed: false, fractionDigits: 1),
            words: range.returnPeriodPhrase,
            direction: PortfolioSummary.Direction(pnlMicros: pnlMicros))
    }

    public static func spoken(
        netWorthMicros: Int64, pnlMicros: Int64, returnBps: Int64, range: LeaderboardRange
    ) -> String {
        let worth = netWorth(micros: netWorthMicros).whole.dropFirst()
        let percent = PercentFormatter.format(basisPoints: abs(returnBps), signed: false, fractionDigits: 1)
            .dropLast()
        let move: String
        switch PortfolioSummary.Direction(pnlMicros: pnlMicros) {
        case .flat: move = "unchanged"
        case .up: move = "up \(dollarsAndCents(pnlMicros)), \(percent) percent"
        case .down: move = "down \(dollarsAndCents(-pnlMicros)), \(percent) percent"
        }
        return "Net worth, \(worth) dollars, \(move), \(range.returnPeriodPhrase)"
    }

    private static func dollarsAndCents(_ micros: Int64) -> String {
        let totalCents = (micros + 5_000) / 10_000
        let dollars = totalCents / 100
        let cents = totalCents % 100
        var parts: [String] = []
        if dollars > 0 || cents == 0 { parts.append("\(dollars) \(dollars == 1 ? "dollar" : "dollars")") }
        if cents > 0 { parts.append("\(cents) \(cents == 1 ? "cent" : "cents")") }
        return parts.joined(separator: " ")
    }
}
