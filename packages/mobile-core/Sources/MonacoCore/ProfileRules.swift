import Foundation

/// Why a display name was rejected. Mirrors the backend `DisplayNameReason` so inline
/// validation and server errors read the same.
public enum DisplayNameValidationError: Error, Equatable, Sendable {
    case required
    case tooLong
    case invalidCharacters
    case needsLetterOrNumber

    public var message: String {
        switch self {
        case .required:
            return "Display name is required."
        case .tooLong:
            return "Display name must be \(DisplayNameRules.maxLength) characters or fewer."
        case .invalidCharacters:
            return "Display name can only use letters, numbers, spaces, punctuation, and emoji."
        case .needsLetterOrNumber:
            return "Display name must include a letter or number."
        }
    }
}

/// Client copy of the `PATCH /v1/me` display name rules
/// (`apps/backend/internal/modules/identity/domain/display_name.go`). The server stays authoritative.
public enum DisplayNameRules {
    public static let minLength = 1
    public static let maxLength = 32
    static let maxConsecutiveMarks = 2
    static let zeroWidthJoiner: UInt32 = 0x200D

    /// Scalars that render as empty space and are used to fake blank names.
    static let blankLookingScalars: Set<UInt32> = [
        0x115F, 0x1160, 0x3164, 0xFFA0, 0x2800, 0x034F, 0x17B4, 0x17B5,
    ]

    /// Canonical form the server will store, or why it will be rejected.
    public static func normalize(_ raw: String) -> Result<String, DisplayNameValidationError> {
        let trimmed = raw.precomposedStringWithCanonicalMapping
            .trimmingCharacters(in: .whitespacesAndNewlines)

        var scalars = String.UnicodeScalarView()
        var pendingSpace = false
        var consecutiveMarks = 0
        var hasLetterOrNumber = false
        var lastIsSymbol = false
        var joinerPending = false

        for scalar in trimmed.unicodeScalars {
            let category = scalar.properties.generalCategory
            if joinerPending && !isSymbol(category) {
                return .failure(.invalidCharacters)
            }
            joinerPending = false
            if scalar.value == zeroWidthJoiner {
                if scalars.isEmpty || pendingSpace || !lastIsSymbol {
                    return .failure(.invalidCharacters)
                }
                scalars.append(scalar)
                joinerPending = true
                consecutiveMarks = 0
                continue
            }
            if scalar == " " || category == .spaceSeparator {
                pendingSpace = !scalars.isEmpty
                consecutiveMarks = 0
                continue
            }
            guard isAllowed(scalar, category: category) else {
                return .failure(.invalidCharacters)
            }
            if isMark(category) {
                consecutiveMarks += 1
                if consecutiveMarks > maxConsecutiveMarks || scalars.isEmpty {
                    return .failure(.invalidCharacters)
                }
            } else {
                consecutiveMarks = 0
                lastIsSymbol = isSymbol(category)
            }
            if pendingSpace {
                scalars.append(" ")
                pendingSpace = false
            }
            if isLetter(category) || isNumber(category) {
                hasLetterOrNumber = true
            }
            scalars.append(scalar)
        }

        return finish(scalars, joinerPending: joinerPending, hasLetterOrNumber: hasLetterOrNumber)
    }

    private static func finish(
        _ scalars: String.UnicodeScalarView,
        joinerPending: Bool,
        hasLetterOrNumber: Bool
    ) -> Result<String, DisplayNameValidationError> {
        let count = scalars.count
        if count < minLength {
            return .failure(.required)
        }
        if count > maxLength {
            return .failure(.tooLong)
        }
        if joinerPending {
            return .failure(.invalidCharacters)
        }
        if !hasLetterOrNumber {
            return .failure(.needsLetterOrNumber)
        }
        return .success(String(scalars))
    }

    /// Inline message for a draft, or nil when it would be accepted.
    public static func validationMessage(for draft: String) -> String? {
        if case .failure(let error) = normalize(draft) {
            return error.message
        }
        return nil
    }

    private static func isAllowed(_ scalar: Unicode.Scalar, category: Unicode.GeneralCategory) -> Bool {
        if blankLookingScalars.contains(scalar.value) {
            return false
        }
        return isLetter(category) || isMark(category) || isNumber(category) || isPunctuation(category)
            || isSymbol(category)
    }

    private static func isLetter(_ category: Unicode.GeneralCategory) -> Bool {
        switch category {
        case .uppercaseLetter, .lowercaseLetter, .titlecaseLetter, .modifierLetter, .otherLetter:
            return true
        default:
            return false
        }
    }

    private static func isMark(_ category: Unicode.GeneralCategory) -> Bool {
        switch category {
        case .nonspacingMark, .spacingMark, .enclosingMark:
            return true
        default:
            return false
        }
    }

    private static func isNumber(_ category: Unicode.GeneralCategory) -> Bool {
        switch category {
        case .decimalNumber, .letterNumber, .otherNumber:
            return true
        default:
            return false
        }
    }

    private static func isPunctuation(_ category: Unicode.GeneralCategory) -> Bool {
        switch category {
        case .connectorPunctuation, .dashPunctuation, .openPunctuation, .closePunctuation,
            .initialPunctuation, .finalPunctuation, .otherPunctuation:
            return true
        default:
            return false
        }
    }

    private static func isSymbol(_ category: Unicode.GeneralCategory) -> Bool {
        switch category {
        case .mathSymbol, .currencySymbol, .modifierSymbol, .otherSymbol:
            return true
        default:
            return false
        }
    }
}

/// Up to two initials for an avatar placeholder: first letters of the first and last words.
public enum AvatarInitials {
    public static func from(_ displayName: String) -> String {
        let words =
            displayName
            .split(whereSeparator: { $0.isWhitespace })
            .compactMap { word in word.first(where: { $0.isLetter || $0.isNumber }) }
        guard let first = words.first else { return "" }
        guard words.count > 1, let last = words.last else {
            return String(first).uppercased()
        }
        return (String(first) + String(last)).uppercased()
    }
}

/// "Member since Sep 2026" in the viewer's calendar.
public enum MemberSinceFormatter {
    public static func format(
        _ date: Date,
        timeZone: TimeZone = .current,
        locale: Locale = .current
    ) -> String {
        let formatter = DateFormatter()
        formatter.locale = locale
        formatter.timeZone = timeZone
        formatter.setLocalizedDateFormatFromTemplate("MMM yyyy")
        return "Member since \(formatter.string(from: date))"
    }
}
