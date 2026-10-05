import Foundation

/// Share or token count for a sell proposal's `tokenAmount` (atomic units), e.g. "0.5".
public enum ProposalShareFormatter {
    public static let defaultDecimals = AssetCatalogDefaults.decimals

    public static func shares(fromAtomics raw: String, decimals: Int = defaultDecimals) -> String {
        guard let quantity = TokenQuantityFormatter.quantity(fromAtomics: raw, decimals: decimals) else { return raw }
        return sharesFormatter(maxFractionDigits: decimals).string(from: quantity as NSDecimalNumber) ?? raw
    }

    private static func sharesFormatter(maxFractionDigits: Int) -> NumberFormatter {
        let formatter = NumberFormatter()
        formatter.locale = Locale(identifier: "en_US")
        formatter.numberStyle = .decimal
        formatter.minimumFractionDigits = 0
        formatter.maximumFractionDigits = maxFractionDigits
        return formatter
    }
}

public enum ProposalTimeFormatter {
    public static func parse(_ raw: String) -> Date? {
        SharedFormatters.iso8601Date(from: raw)
    }

    /// Time left on an open vote, e.g. "Closes in 2d", "Closes in 20h", "Closes in 12m".
    /// Nil when the timestamp is unreadable.
    public static func closesLabel(expiresAt raw: String, now: Date = Date()) -> String? {
        guard let expiry = parse(raw) else { return nil }
        let remaining = Int(expiry.timeIntervalSince(now))
        if remaining <= 0 { return "Voting closed" }
        let days = remaining / 86_400
        let hours = remaining / 3600
        let minutes = remaining / 60
        if days >= 2 { return "Closes in \(days)d" }
        if hours > 0 { return "Closes in \(hours)h" }
        return "Closes in \(max(minutes, 1))m"
    }

    /// True in the last hour of an open vote, when the card flags the deadline.
    public static func closesSoon(expiresAt raw: String, now: Date = Date()) -> Bool {
        guard let expiry = parse(raw) else { return false }
        let remaining = expiry.timeIntervalSince(now)
        return remaining > 0 && remaining < 3600
    }

    /// Compact age for comments and cards, e.g. "now", "12m", "3h", "4d", then a short date.
    public static func ageLabel(_ raw: String, now: Date = Date(), calendar: Calendar = .current) -> String {
        guard let date = parse(raw) else { return "" }
        let elapsed = Int(now.timeIntervalSince(date))
        if elapsed < 60 { return "now" }
        if elapsed < 3600 { return "\(elapsed / 60)m" }
        if elapsed < 86_400 { return "\(elapsed / 3600)h" }
        if elapsed < 7 * 86_400 { return "\(elapsed / 86_400)d" }
        return SharedFormatters.string(from: date, pattern: .template("MMMd"), locale: .current, calendar: calendar)
    }
}
