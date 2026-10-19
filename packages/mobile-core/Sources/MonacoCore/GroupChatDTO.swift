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
    public static let emptyTitle = "Start the conversation"
    public static let emptyHint =
        "Messages here stay with your cabal. Proposal-specific discussion stays attached to the proposal."
    public static let composerPlaceholder = "Message your cabal"
    public static let loadEarlier = "Load earlier messages"
    public static let loadFailure = "Couldn't load messages."
    public static let closed = "You're not in this cabal anymore."
    public static let notSent = "Not sent"
    public static let retry = "Retry"
    public static let discuss = "Discuss"
    public static let sent = "Sent"
    public static let sending = "Sending…"
    public static let offline = "You're offline. Messages can be drafted but won't send yet."
    public static let infoLabel = "Cabal information"
    public static let sendLabel = "Send message"
    public static let review = "Review"
    public static let status = "Status"
    public static let executionPending = "Execution pending · Not completed"
    public static let unknownProposal = "A proposal"
    public static let approved = "Proposal approved"
    public static let deleted = "Message deleted"
    public static let copy = "Copy"

    public static func title(groupName: String?) -> String {
        let trimmed = groupName?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? title : trimmed
    }

    public static func newMessagesPill(count: Int) -> String {
        count == 1 ? "1 new message" : "\(count) new messages"
    }

    public static func proposed(_ name: String, _ asset: String, isSell: Bool = false) -> String {
        isSell ? "\(name) proposed selling \(asset)" : "\(name) proposed \(asset)"
    }

    public static func fromPot(_ amount: String) -> String {
        "\(amount) from the cabal pot"
    }

    public static func statusLine(time: String, status: String) -> String {
        "\(time) · \(status)"
    }

    public static func replies(_ count: Int) -> String {
        count == 1 ? "1 reply" : "\(count) replies"
    }
}
