import Foundation

public enum AnalyticsPII {
    public static let droppedKeys: Set<String> = [
        "email", "phone", "x_handle", "handle", "display_name", "wallet", "address",
    ]

    private static let droppedKeySuffixes = ["_email", "_phone", "_handle", "_wallet", "_address"]
    private static let base58Alphabet = Set("123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz")
    private static let tokenPunctuation = Set(",;:.!?()<>\"'[]{}")

    public static func scrub(_ properties: [String: AnalyticsValue]) -> [String: AnalyticsValue] {
        properties.filter { key, value in
            !isPIIKey(key) && !isPIIValue(value)
        }
    }

    public static func isPIIKey(_ key: String) -> Bool {
        let normalized = key.lowercased().replacingOccurrences(of: "-", with: "_")
        return droppedKeys.contains(normalized) || droppedKeySuffixes.contains { normalized.hasSuffix($0) }
    }

    public static func isPIIValue(_ value: AnalyticsValue) -> Bool {
        guard case .string(let text) = value else { return false }
        return text.split(whereSeparator: \.isWhitespace).contains { token in
            let trimmed = trimPunctuation(token)
            return looksLikeEmail(trimmed) || looksLikePhone(trimmed) || looksLikeBase58Key(trimmed)
        }
    }

    private static func trimPunctuation(_ token: Substring) -> String {
        var characters = Array(token)
        while let first = characters.first, tokenPunctuation.contains(first) { characters.removeFirst() }
        while let last = characters.last, tokenPunctuation.contains(last) { characters.removeLast() }
        return String(characters)
    }

    private static func looksLikeEmail(_ token: String) -> Bool {
        let parts = token.split(separator: "@", omittingEmptySubsequences: false)
        guard parts.count == 2, !parts[0].isEmpty else { return false }
        let labels = parts[1].split(separator: ".", omittingEmptySubsequences: false)
        return labels.count >= 2 && labels.allSatisfy { !$0.isEmpty }
    }

    private static func looksLikePhone(_ token: String) -> Bool {
        guard token.first == "+" else { return false }
        let digits = token.dropFirst()
        return (7...15).contains(digits.count) && digits.first != "0" && digits.allSatisfy(\.isASCIIDigit)
    }

    private static func looksLikeBase58Key(_ token: String) -> Bool {
        (32...44).contains(token.count) && token.allSatisfy { base58Alphabet.contains($0) }
    }
}

extension Character {
    fileprivate var isASCIIDigit: Bool { ("0"..."9").contains(self) }
}
