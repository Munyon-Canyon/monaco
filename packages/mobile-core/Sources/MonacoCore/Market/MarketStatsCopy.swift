import Foundation
import MonacoAPI

public enum MarketStatsCopy {
    public struct Row: Equatable, Sendable {
        public let label: String
        public let value: String

        public init(label: String, value: String) {
            self.label = label
            self.value = value
        }
    }

    public struct Sheet: Equatable, Sendable {
        public let title: String
        public let message: String
        public let rows: [Row]
    }

    public static let title = "Market stats"
    public static let estimateTitle = "Pre-IPO estimate"
    public static let estimateMessage =
        "Last price paid between investors. Not a live price."
    public static let sourceLabel = "Source"
    public static let updatedLabel = "Updated"
    public static let delayLabel = "Delay"
    public static let noSource = "—"
    public static let notPriced = "Not priced yet"
    public static let continuousMessage =
        "Monaco prices trade around the clock, so they can differ a little from the stock market's."

    public static func message(_ facts: MarketStatsFacts) -> String { continuousMessage }

    public static func rows(_ facts: MarketStatsFacts, now: Date, calendar: Calendar = .autoupdatingCurrent) -> [Row] {
        [
            Row(label: sourceLabel, value: sourceName(facts.priceSource)),
            Row(label: updatedLabel, value: updated(facts.priceAsOf, now: now, calendar: calendar)),
            Row(label: delayLabel, value: delay(facts.pollInterval)),
        ]
    }

    public static func sheet(_ facts: MarketStatsFacts, now: Date, calendar: Calendar = .autoupdatingCurrent) -> Sheet {
        Sheet(title: title, message: message(facts), rows: rows(facts, now: now, calendar: calendar))
    }

    public static func estimateSheet(_ preIpo: PreIpoDetail) -> Sheet {
        Sheet(
            title: estimateTitle, message: estimateMessage,
            rows: (preIpo.profile?.ledger ?? []).map { Row(label: $0.label, value: $0.value) })
    }

    public static func sourceName(_ source: PriceSource?) -> String {
        switch source {
        case .jupiter: "Jupiter"
        case .coingecko: "CoinGecko"
        case nil: noSource
        }
    }

    static func updated(_ date: Date?, now: Date, calendar: Calendar) -> String {
        guard let date else { return notPriced }
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.calendar = calendar
        formatter.timeZone = calendar.timeZone
        formatter.dateFormat = calendar.isDate(date, inSameDayAs: now) ? "h:mm a" : "MMM d"
        return formatter.string(from: date)
    }

    static func delay(_ interval: Duration) -> String {
        let seconds = Int(interval.components.seconds)
        if seconds == 60 { return "up to 1 min" }
        if seconds % 60 == 0 { return "up to \(seconds / 60) min" }
        return "up to \(seconds) s"
    }

    public static let auditedStrings: [String] = [
        title, estimateTitle, estimateMessage, sourceLabel, updatedLabel, delayLabel, noSource, notPriced,
        continuousMessage,
        "Jupiter", "CoinGecko", "up to 1 min", "up to 2 min", "up to 30 s",
    ]
}
