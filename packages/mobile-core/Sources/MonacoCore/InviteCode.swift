public struct InviteCode: Hashable, Sendable, CustomStringConvertible {
    public static let length = 10

    public let value: String

    public init?(_ raw: String) {
        let code = String(
            raw.trimmingCharacters(in: .whitespacesAndNewlines).uppercased().map(Self.crockfordDigit)
        )
        guard code.count == Self.length, code.allSatisfy(Self.alphabet.contains) else { return nil }
        value = code
    }

    public var description: String { value }

    private static let alphabet = Set("0123456789ABCDEFGHJKMNPQRSTVWXYZ")

    private static func crockfordDigit(_ character: Character) -> Character {
        switch character {
        case "I", "L": "1"
        case "O": "0"
        default: character
        }
    }
}
