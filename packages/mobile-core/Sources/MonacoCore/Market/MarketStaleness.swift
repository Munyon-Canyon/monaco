import Foundation

public enum MarketStaleness {
    public static let priceAgeLimit: TimeInterval = 5 * 60

    public static func isDelayed(
        serverStale: Bool?, priceAsOf: Date?, session: MarketSession, now: Date
    ) -> Bool {
        if serverStale == true { return true }
        guard session == .open, let priceAsOf else { return false }
        return now.timeIntervalSince(priceAsOf) > priceAgeLimit
    }

    public static func minutes(from earlier: Date, to later: Date) -> Int {
        max(0, Int(later.timeIntervalSince(earlier) / 60))
    }

    public static func ago(minutes: Int) -> String {
        switch minutes {
        case ..<1: "just now"
        case 1: "1 minute ago"
        default: "\(minutes) minutes ago"
        }
    }

    public static func pricesDelayed(minutes: Int) -> String {
        "Prices delayed · \(ago(minutes: minutes))."
    }
}
