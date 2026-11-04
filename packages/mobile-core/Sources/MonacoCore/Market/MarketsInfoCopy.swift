import Foundation

public enum MarketsInfoCopy {
    public static let preIpoTitle = "Pre-IPO"

    public static let preIpo =
        "Pre-IPO companies are private. Their price is a secondary estimate between funding rounds, not a live quote. You can propose to buy the ones marked Can buy; the ones marked Watch only can't be bought here yet."
}

public enum PreIpoRowCopy {
    private static let months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

    public static func estimateLine(asOf: String) -> String {
        "Secondary estimate · \(day(asOf))"
    }

    private static let monthNames = [
        "January", "February", "March", "April", "May", "June",
        "July", "August", "September", "October", "November", "December",
    ]

    public static func roundLine(
        valuationMicros: String?, lastRoundOn: String?, estimateAsOf: String?, ticker: String, today: Date
    ) -> String {
        guard let valuationMicros, let amount = AssetStats.abbreviated(valuationMicros) else {
            return estimateAsOf.map(estimateLine(asOf:)) ?? ticker
        }
        let valued = "Valued at \(amount)"
        guard let lastRoundOn, let round = roundMonth(lastRoundOn) else { return valued }
        let name = monthNames[round.month - 1]
        return roundIsStale(round, today: today) ? "\(valued) in \(name) \(round.year)" : "\(valued) in \(name)"
    }

    private static func roundMonth(_ iso: String) -> (year: Int, month: Int)? {
        let parts = iso.split(separator: "-")
        guard parts.count == 3, let year = Int(parts[0]), let month = Int(parts[1]), (1...12).contains(month) else {
            return nil
        }
        return (year, month)
    }

    private static func roundIsStale(_ round: (year: Int, month: Int), today: Date) -> Bool {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        let now = calendar.dateComponents([.year, .month], from: today)
        guard let year = now.year, let month = now.month else { return false }
        return (year * 12 + month) - (round.year * 12 + round.month) > 11
    }

    public static func eligibility(tradable: Bool) -> String {
        tradable ? "Can buy" : "Watch only"
    }

    public static func footer(asOfDates: [String]) -> String? {
        let sorted = asOfDates.sorted()
        guard let first = sorted.first, let last = sorted.last else { return nil }
        let prefix = "Prices are secondary-market estimates from "
        return first == last ? "\(prefix)\(day(first))." : "\(prefix)\(day(first)) to \(day(last))."
    }

    static func day(_ iso: String) -> String {
        let parts = iso.split(separator: "-")
        guard parts.count == 3, let month = Int(parts[1]), let day = Int(parts[2]), (1...12).contains(month) else {
            return iso
        }
        return "\(months[month - 1]) \(day)"
    }
}
