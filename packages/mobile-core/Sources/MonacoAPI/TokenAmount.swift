import Foundation

public enum TokenAmount {
    private static let posix = Locale(identifier: "en_US_POSIX")

    public static func parse(_ text: String) -> Decimal? {
        guard !text.isEmpty, text.utf8.allSatisfy({ $0 >= 48 && $0 <= 57 }) else { return nil }
        return Decimal(string: text, locale: posix)
    }

    public static func decimal(_ text: String?, fallback: Int64?) -> Decimal? {
        if let text, let value = parse(text) { return value }
        return fallback.map { Decimal($0) }
    }

    public static func text(_ amount: Decimal) -> String {
        NSDecimalNumber(decimal: rounded(amount, .down)).stringValue
    }

    public static func of(_ value: Int64) -> Decimal { Decimal(value) }

    public static func scale(_ decimals: Int) -> Decimal {
        Decimal(sign: .plus, exponent: decimals, significand: 1)
    }

    public static func rounded(_ value: Decimal, _ mode: NSDecimalNumber.RoundingMode) -> Decimal {
        var whole = Decimal()
        var copy = value
        NSDecimalRound(&whole, &copy, 0, mode)
        return whole
    }

    public static func int64(_ amount: Decimal) -> Int64? {
        Int64(text(amount))
    }
}
