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
