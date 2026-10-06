import Foundation

public struct E164PhoneNumber: Equatable, Sendable {
    public let value: String

    public init?(_ input: String, defaultRegion: String = "US") {
        guard let scanned = Self.scan(input) else { return nil }
        guard
            let national = Self.nationalNumber(
                digits: scanned.digits, leadsWithPlus: scanned.leadsWithPlus, defaultRegion: defaultRegion)
        else { return nil }
        guard (8...15).contains(national.count), !national.hasPrefix("0") else { return nil }
        value = "+" + national
    }

    public var displayValue: String {
        let digits = value.dropFirst()
        guard digits.hasPrefix("1"), digits.count == 11 else { return value }
        let national = Array(digits.dropFirst())
        let area = String(national[0..<3])
        let prefix = String(national[3..<6])
        let line = String(national[6..<10])
        return "(\(area)) \(prefix)-\(line)"
    }

    private static func isNANPRegion(_ region: String) -> Bool {
        let code = region.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
        return code == "US" || code == "CA"
    }

    private static func isNANPNational(_ digits: String) -> Bool {
        guard digits.count == 10, let first = digits.first else { return false }
        return ("2"..."9").contains(first)
    }

    private static func nationalNumber(digits: String, leadsWithPlus: Bool, defaultRegion: String) -> String? {
        if leadsWithPlus { return digits }
        if isNANPRegion(defaultRegion) {
            if isNANPNational(digits) { return "1" + digits }
            if digits.count == 11, digits.hasPrefix("1"), isNANPNational(String(digits.dropFirst())) {
                return digits
            }
        }
        if digits.hasPrefix("00") { return String(digits.dropFirst(2)) }
        return nil
    }

    private static func scan(_ input: String) -> (digits: String, leadsWithPlus: Bool)? {
        var digits = ""
        var leadsWithPlus = false
        for scalar in input.unicodeScalars {
            if CharacterSet.asciiDigits.contains(scalar) {
                digits.unicodeScalars.append(scalar)
            } else if scalar == "+" {
                guard digits.isEmpty, !leadsWithPlus else { return nil }
                leadsWithPlus = true
            } else if !CharacterSet.phoneFormatting.contains(scalar) {
                return nil
            }
        }
        guard !digits.isEmpty else { return nil }
        return (digits, leadsWithPlus)
    }
}

extension CharacterSet {
    fileprivate static let asciiDigits = CharacterSet(charactersIn: "0123456789")

    fileprivate static let phoneFormatting = CharacterSet(charactersIn: "()-./ \t")
        .union(CharacterSet(charactersIn: "\u{00A0}\u{2009}\u{202F}"))
        .union(CharacterSet(charactersIn: "\u{2013}\u{2014}"))
        .union(CharacterSet(charactersIn: "\u{200E}\u{200F}\u{061C}"))
        .union(CharacterSet(charactersIn: "\u{202A}\u{202B}\u{202C}\u{202D}\u{202E}"))
        .union(CharacterSet(charactersIn: "\u{2066}\u{2067}\u{2068}\u{2069}"))
}
