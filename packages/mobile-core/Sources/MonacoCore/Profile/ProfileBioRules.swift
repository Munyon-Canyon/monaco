import Foundation

public enum ProfileBioRules {
    public static let maxScalars = 160

    public static func normalized(_ raw: String) -> String {
        raw.precomposedStringWithCanonicalMapping.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public static func scalarCount(_ raw: String) -> Int {
        normalized(raw).unicodeScalars.count
    }

    public static func validationMessage(for raw: String) -> String? {
        let value = normalized(raw)
        if value.unicodeScalars.contains(where: { $0 != "\n" && $0.properties.generalCategory == .control }) {
            return "Bio can't use control characters."
        }
        if value.unicodeScalars.count > maxScalars {
            return "Bio must be \(maxScalars) characters or fewer."
        }
        return nil
    }

    public static func remaining(_ raw: String) -> Int {
        maxScalars - scalarCount(raw)
    }
}
