import Foundation

public enum AmountKey: Hashable, Sendable {
    case digit(Int)
    case dot
    case delete
}

public enum AmountText {
    private static let posix = Locale(identifier: "en_US_POSIX")

    public static func applying(_ key: AmountKey, to text: String) -> String {
        switch key {
        case .digit(let digit):
            return sanitize(text + String(digit))
        case .dot:
            return sanitize(text + ".")
        case .delete:
            return String(text.dropLast())
        }
    }

    public static func sanitize(_ raw: String) -> String {
        let normalised = normalisingSeparators(raw)
        var result = ""
        var seenDot = false
        var decimals = 0
        for character in normalised {
            if character.isASCII, character.isNumber {
                if seenDot {
                    guard decimals < 2 else { continue }
                    decimals += 1
                }
                result.append(character)
            } else if character == ".", !seenDot {
                seenDot = true
                result.append(result.isEmpty ? "0." : ".")
            }
        }
        while result.count > 1, result.hasPrefix("0"), !result.hasPrefix("0.") {
            result.removeFirst()
        }
        if result.count > 12 { result = String(result.prefix(12)) }
        return result
    }

    private static func normalisingSeparators(_ raw: String) -> String {
        let hasComma = raw.contains(",")
        guard hasComma else { return raw }

        if let lastComma = raw.lastIndex(of: ","), let lastDot = raw.lastIndex(of: ".") {
            if lastComma > lastDot {
                return raw.replacingOccurrences(of: ".", with: "").replacingOccurrences(of: ",", with: ".")
            }
            return raw.replacingOccurrences(of: ",", with: "")
        }

        if commasAreGrouping(raw) {
            return raw.replacingOccurrences(of: ",", with: "")
        }
        return raw.replacingOccurrences(of: ",", with: ".")
    }

    private static func commasAreGrouping(_ raw: String) -> Bool {
        let compact = raw.filter { ($0.isASCII && $0.isNumber) || $0 == "," }
        let groups = compact.split(separator: ",", omittingEmptySubsequences: false)
        guard groups.count >= 2, (1...3).contains(groups[0].count) else { return false }
        return groups.dropFirst().allSatisfy { $0.count == 3 }
    }

    public static func decimal(_ text: String) -> Decimal? {
        guard !text.isEmpty else { return nil }
        return Decimal(string: text, locale: posix)
    }

    public static func micros(_ text: String) -> Int64? {
        guard let value = decimal(text), value >= 0 else { return nil }
        var scaled = value * 1_000_000
        var rounded: Decimal = 0
        NSDecimalRound(&rounded, &scaled, 0, .plain)
        return (rounded as NSDecimalNumber).int64Value
    }

    public static func dollars(micros: Int64) -> Decimal {
        Decimal(micros) / 1_000_000
    }

    public static func display(_ text: String) -> String {
        guard !text.isEmpty else { return "$0" }
        let parts = text.split(separator: ".", omittingEmptySubsequences: false)
        let integer = Int(parts[0]) ?? 0
        let integerText = groupedIntegerFormatter.string(from: NSNumber(value: integer)) ?? String(parts[0])
        if parts.count > 1 {
            return "$\(integerText).\(parts[1])"
        }
        return "$\(integerText)"
    }

    public static func adding(_ value: Decimal, to text: String) -> String {
        plain((decimal(text) ?? 0) + value)
    }

    public static func plain(_ decimal: Decimal) -> String {
        plainFormatter.string(from: decimal as NSDecimalNumber) ?? "\(decimal)"
    }

    private static let groupedIntegerFormatter: NumberFormatter = {
        let formatter = NumberFormatter()
        formatter.locale = posix
        formatter.numberStyle = .decimal
        formatter.usesGroupingSeparator = true
        formatter.groupingSeparator = ","
        return formatter
    }()

    private static let plainFormatter: NumberFormatter = {
        let formatter = NumberFormatter()
        formatter.locale = posix
        formatter.numberStyle = .decimal
        formatter.usesGroupingSeparator = false
        formatter.minimumFractionDigits = 0
        formatter.maximumFractionDigits = 2
        return formatter
    }()

    public static func presetRows(_ count: Int, stacked: Bool) -> [[Int]] {
        guard count > 0 else { return [] }
        guard stacked, count > 2 else { return [Array(0..<count)] }
        let split = (count + 1) / 2
        return [Array(0..<split), Array(split..<count)]
    }

    public static func roundDownToCents(_ decimal: Decimal) -> Decimal {
        var source = decimal
        var result = Decimal()
        NSDecimalRound(&result, &source, 2, .down)
        return result
    }
}
