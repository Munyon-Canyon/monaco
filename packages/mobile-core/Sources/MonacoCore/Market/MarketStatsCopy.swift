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
    public static let estimateTitle = "Secondary estimate"
    public static let estimateMessage =
        "Pre-IPO companies don't trade on an exchange. The estimate is what secondary buyers paid recently."
    public static let sourceLabel = "Source"
    public static let updatedLabel = "Updated"
    public static let delayLabel = "Delay"
    public static let noSource = "—"
    public static let notPriced = "Not priced yet"
    public static let ok = "OK"
    static let continuousTail = "It trades 24/7."

    public static func message(_ facts: MarketStatsFacts) -> String {
        let chains = joined(facts.listingNetworks.map(BestPrice.chainName))
        return "Prices are for the tokenized stock on \(chains), not the exchange listing. \(continuousTail)"
    }

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

    private static func joined(_ names: [String]) -> String {
        guard let last = names.last, names.count > 1 else { return names.first ?? "" }
        return names.dropLast().joined(separator: ", ") + " and " + last
    }

    public static let auditedStrings: [String] = [
        title, estimateTitle, estimateMessage, sourceLabel, updatedLabel, delayLabel, noSource, notPriced, ok,
        "Prices are for the tokenized stock on Solana, not the exchange listing. It trades 24/7.",
        "Jupiter", "CoinGecko", "up to 1 min", "up to 2 min", "up to 30 s",
    ]
}
