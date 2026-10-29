public enum MarketsInfoCopy {
    public static let preIpoTitle = "Pre-IPO"

    public static let preIpo =
        "Pre-IPO companies are private. Their price is a secondary estimate between funding rounds, not a live quote. Eligible companies can be proposed; Research only ones can't be bought here yet."
}

public enum PreIpoRowCopy {
    private static let months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

    public static func estimateLine(asOf: String) -> String {
        "Secondary estimate · \(day(asOf))"
    }

    public static func eligibility(tradable: Bool) -> String {
        tradable ? "Eligible" : "Research only"
    }

    static func day(_ iso: String) -> String {
        let parts = iso.split(separator: "-")
        guard parts.count == 3, let month = Int(parts[1]), let day = Int(parts[2]), (1...12).contains(month) else {
            return iso
        }
        return "\(months[month - 1]) \(day)"
    }
}
