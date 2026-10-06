import Foundation

enum GroupChatDates {
    static func parse(_ value: String) -> Date? {
        if let date = SharedFormatters.iso8601WholeSeconds.date(from: value) { return date }
        return SharedFormatters.iso8601Fractional.date(from: truncatingFraction(value, toDigits: 3))
    }

    private static func truncatingFraction(_ value: String, toDigits digits: Int) -> String {
        guard let dot = value.firstIndex(of: ".") else { return value }
        let fractionStart = value.index(after: dot)
        let fractionEnd = value[fractionStart...].firstIndex { !$0.isNumber } ?? value.endIndex
        let fraction = value[fractionStart..<fractionEnd]
        guard fraction.count > digits else { return value }
        return String(value[..<fractionStart]) + fraction.prefix(digits) + value[fractionEnd...]
    }
}

public enum GroupChatDraft {
    public static let maxCharacters = 2000

    public enum Problem: Error, Equatable, Sendable {
        case empty
        case tooLong(count: Int)
    }

    public static func validate(_ text: String) -> Result<String, Problem> {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return .failure(.empty) }
        let count = trimmed.unicodeScalars.count
        if count > maxCharacters { return .failure(.tooLong(count: count)) }
        return .success(trimmed)
    }
}

public enum GroupChatCopy {
    public static let title = "Cabal chat"
    public static let emptyState =
        "No messages yet. Say hi to your cabal or float a stock idea before someone proposes a buy."
    public static let composerPlaceholder = "Message your cabal"
    public static let loadEarlier = "Load earlier messages"
    public static let loadFailure = "Couldn't load messages."
    public static let closed = "You're no longer in this cabal, so its chat is closed to you."
    public static let notSent = "Not sent · Retry"
    public static let deleted = "Message deleted"

    public static func title(groupName: String?) -> String {
        let trimmed = groupName?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? title : trimmed
    }

    public static func newMessagesPill(count: Int) -> String {
        count == 1 ? "1 new message" : "\(count) new messages"
    }

    public static func replies(_ count: Int) -> String {
        count == 1 ? "1 reply" : "\(count) replies"
    }
}
