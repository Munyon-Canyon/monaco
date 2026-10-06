import Foundation

/// One cabal chat message from `GET/POST /v1/groups/{id}/messages`.
public struct GroupMessageDTO: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let groupId: String
    public let authorId: String
    public let authorName: String
    public let body: String
    /// Fixed-width RFC3339 UTC timestamp with microseconds, e.g. `2026-09-18T15:04:05.123456Z`.
    public let createdAt: String
    /// True when the signed-in viewer wrote this message.
    public let mine: Bool

    public init(
        id: String,
        groupId: String,
        authorId: String,
        authorName: String,
        body: String,
        createdAt: String,
        mine: Bool
    ) {
        self.id = id
        self.groupId = groupId
        self.authorId = authorId
        self.authorName = authorName
        self.body = body
        self.createdAt = createdAt
        self.mine = mine
    }

    public var createdAtDate: Date? {
        GroupChatDates.parse(createdAt)
    }
}

/// Newest-first page. `nextCursor` is present only when older messages exist.
public struct GroupMessagesPageDTO: Codable, Equatable, Sendable {
    public let messages: [GroupMessageDTO]
    public let nextCursor: String?

    public init(messages: [GroupMessageDTO], nextCursor: String? = nil) {
        self.messages = messages
        self.nextCursor = nextCursor
    }
}

enum GroupChatDates {
    static func parse(_ value: String) -> Date? {
        if let date = SharedFormatters.iso8601WholeSeconds.date(from: value) { return date }
        return SharedFormatters.iso8601Fractional.date(from: truncatingFraction(value, toDigits: 3))
    }

    /// ISO8601DateFormatter only reliably reads millisecond fractions; drop digits past that.
    private static func truncatingFraction(_ value: String, toDigits digits: Int) -> String {
        guard let dot = value.firstIndex(of: ".") else { return value }
        let fractionStart = value.index(after: dot)
        let fractionEnd = value[fractionStart...].firstIndex { !$0.isNumber } ?? value.endIndex
        let fraction = value[fractionStart..<fractionEnd]
        guard fraction.count > digits else { return value }
        return String(value[..<fractionStart]) + fraction.prefix(digits) + value[fractionEnd...]
    }
}

/// Chat transport used by the chat screen. `MonacoAPIClient` is the live implementation.
public protocol GroupChatService: Sendable {
    func listGroupMessages(groupId: String, before: String?, limit: Int) async throws -> GroupMessagesPageDTO
    func postGroupMessage(groupId: String, body: String) async throws -> GroupMessageDTO
}

extension MonacoAPIClient: GroupChatService {}

/// Client-side draft rules mirroring the API (trimmed, 1...2000 characters).
public enum GroupChatDraft {
    public static let maxCharacters = 2000

    public enum Problem: Error, Equatable, Sendable {
        case empty
        case tooLong(count: Int)
    }

    /// Returns the trimmed body to send, or the reason it cannot be sent.
    public static func validate(_ text: String) -> Result<String, Problem> {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return .failure(.empty) }
        let count = trimmed.unicodeScalars.count
        if count > maxCharacters { return .failure(.tooLong(count: count)) }
        return .success(trimmed)
    }
}

/// User-facing chat copy shared by the app and copy audits.
public enum GroupChatCopy {
    /// Fallback navigation title when the cabal's name isn't known yet.
    public static let title = "Cabal chat"
    public static let emptyState =
        "No messages yet. Say hi to your cabal or float a stock idea before someone proposes a buy."
    public static let composerPlaceholder = "Message your cabal"
    public static let loadEarlier = "Load earlier messages"
    public static let loadFailure = "Couldn't load messages."
    public static let closed = "You're no longer in this cabal, so its chat is closed to you."
    public static let notSent = "Not sent · Retry"
    public static let deleted = "Message deleted"

    public static func sendFailure(_ error: Error) -> String {
        switch error {
        case GroupChatDraft.Problem.empty:
            return "Type a message first."
        case GroupChatDraft.Problem.tooLong:
            return "Messages can be up to \(GroupChatDraft.maxCharacters) characters."
        // The chat routes map their failures in full, so a 429 arrives as its own case
        // carrying the server's Retry-After.
        case MonacoAPIError.rateLimited(let retryAfterSeconds, _):
            guard let seconds = retryAfterSeconds, seconds > 0 else {
                return "You're sending messages fast. Wait a moment and try again."
            }
            return "You're sending messages fast. Try again in \(seconds) second\(seconds == 1 ? "" : "s")."
        // 4xx bodies carry the API's own reason; show it when it was written for members.
        case MonacoAPIError.rejected(let status, let message, _):
            if let memberFacing = MoneyFlowCopy.memberFacingMessage(message) { return memberFacing }
            return sendFailure(MonacoAPIError.httpStatus(status))
        // The API answered with something we can't read, which is no evidence it didn't store
        // the message first.
        case MonacoAPIError.invalidResponse:
            return sendUnconfirmed
        case MonacoAPIError.httpStatus(let code, _):
            switch code {
            case 401: return "Your session expired. Sign in again to chat."
            case 403: return "Only members of this cabal can chat here."
            case 404: return "This cabal no longer exists."
            case 429: return "You're sending messages fast. Wait a moment and try again."
            case 400: return "That message couldn't be sent. Check the text and try again."
            // The request reached the API and it broke on its own side of the line. That says
            // nothing about whether it wrote the message down before it did.
            case 500...599: return sendUnconfirmed
            default: return "Message not sent. Try again."
            }
        default:
            // Not a URL error at all: it got as far as a reply we couldn't read. A 201 whose
            // body fails to decode is still a message the API stored.
            guard let urlError = error as? URLError else { return sendUnconfirmed }
            if urlError.code == .notConnectedToInternet { return "You're offline. Message not sent." }
            return FlowErrorInput.neverSentURLErrorCodes.contains(urlError.code)
                ? "Message not sent. Check your connection and try again."
                : sendUnconfirmed
        }
    }

    /// Sending posts a bare body — no idempotency key, unlike the money routes — so a second
    /// attempt at a message that did land posts it twice, and chat has no delete. When the
    /// failure could only have happened after the request went out, say we don't know instead
    /// of promising it didn't arrive; the next poll answers the question.
    ///
    /// Same judgement the money flows make in `MoneyFlowCopy.unconfirmed`, and the same list of
    /// URL errors raised before any byte leaves the device.
    public static let sendUnconfirmed = "We couldn't confirm that went through. Check above before sending it again."

    /// True when a failed send leaves it unknown whether the API stored the message.
    ///
    /// The same judgement `sendFailure` already makes, asked as a question rather than
    /// restated, so the two cannot drift apart. The caller needs it because the two answers
    /// differ in what to do with the member's text: a send that definitely failed should hand
    /// it back, while one that may have landed must not — chat has no delete, and a composer
    /// refilled with a message that is already in the thread is one tap from the duplicate
    /// this sentence exists to prevent.
    public static func isSendUnconfirmed(_ error: Error) -> Bool {
        sendFailure(error) == sendUnconfirmed
    }

    /// The chat screen is titled with the cabal's own name; "Cabal chat" only when it's missing.
    public static func title(groupName: String?) -> String {
        let trimmed = groupName?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? title : trimmed
    }

    /// Gap after which the thread shows a centred time separator instead of stamping every bubble.
    public static let timeSeparatorGap: TimeInterval = 10 * 60

    /// True for the first message and whenever more than ten minutes passed since the previous one.
    public static func showsTimeSeparator(previous: Date?, current: Date) -> Bool {
        guard let previous else { return true }
        return current.timeIntervalSince(previous) > timeSeparatorGap
    }

    /// "Today 12:40", "Yesterday 9:02 AM", "Sep 14, 9:02 AM" in the viewer's local time (the API stores UTC).
    public static func timeSeparatorLabel(
        _ date: Date,
        now: Date = Date(),
        calendar: Calendar = .current,
        locale: Locale = .current
    ) -> String {
        let clock = SharedFormatters.string(from: date, pattern: .template("jmm"), locale: locale, calendar: calendar)
        if calendar.isDate(date, inSameDayAs: now) {
            return "Today \(clock)"
        }
        if let yesterday = calendar.date(byAdding: .day, value: -1, to: now),
            calendar.isDate(date, inSameDayAs: yesterday)
        {
            return "Yesterday \(clock)"
        }
        let sameYear = calendar.component(.year, from: date) == calendar.component(.year, from: now)
        let day = SharedFormatters.string(
            from: date,
            pattern: .template(sameYear ? "MMMd" : "yMMMd"),
            locale: locale,
            calendar: calendar
        )
        return "\(day), \(clock)"
    }

    /// Shown in place of the thread when the first page never arrived. A Try again button
    /// sits right under it, so this must not send the member looking for a gesture: there is
    /// nothing to pull on an empty screen.
    public static func loadFailure(_ error: Error) -> String {
        if let closed = chatClosed(error) { return closed }
        return "Couldn't load messages."
    }

    /// Toast for a refresh of a thread already on screen, where pulling down does work.
    public static func refreshFailure(_ error: Error) -> String {
        if let closed = chatClosed(error) { return closed }
        return "Couldn't refresh messages. Pull down to try again."
    }

    /// Toast for the "Load earlier messages" button.
    public static func earlierFailure(_ error: Error) -> String {
        if let closed = chatClosed(error) { return closed }
        return "Couldn't load earlier messages. Try again."
    }

    /// Why this thread is no longer readable, or nil for a failure worth retrying. Chat stops
    /// polling on one of these instead of asking a cabal it was thrown out of every 4 seconds.
    public static func chatClosed(_ error: Error) -> String? {
        switch error {
        case MonacoAPIError.httpStatus(let code, _):
            return chatClosed(status: code, serverMessage: nil)
        case MonacoAPIError.rejected(let code, let message, _):
            return chatClosed(status: code, serverMessage: message)
        default:
            return nil
        }
    }

    /// 403 is the API making a decision about this member, and it means what it says.
    ///
    /// 404 does not. The messages route answers 404 for a missing user and a missing path id
    /// as well as a missing cabal, so reading every 404 as "your cabal is gone" tells a member
    /// their cabal was deleted because a membership read landed on a replica that had not
    /// caught up yet. The body is the only thing that tells them apart — these branches log a
    /// machine-readable reason but do not put it in the response — so match the one that is
    /// about something other than the group and treat it as worth retrying.
    private static func chatClosed(status: Int, serverMessage: String?) -> String? {
        switch status {
        case 403:
            return "You're no longer in this cabal, so its chat is closed to you."
        case 404:
            guard !isAboutTheUser(serverMessage) else { return nil }
            return "This cabal no longer exists."
        default:
            return nil
        }
    }

    private static func isAboutTheUser(_ serverMessage: String?) -> Bool {
        guard let serverMessage else { return false }
        return serverMessage.localizedCaseInsensitiveContains("user not found")
    }

    public static func replies(_ count: Int) -> String {
        count == 1 ? "1 reply" : "\(count) replies"
    }

    /// The pill offered to a member reading history when messages land below them.
    public static func newMessagesPill(count: Int) -> String {
        count == 1 ? "1 new message" : "\(count) new messages"
    }
}
