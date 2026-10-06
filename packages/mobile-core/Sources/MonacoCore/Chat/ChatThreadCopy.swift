import Foundation

public enum ChatThreadCopy {
    public static let title = "Thread"
    public static let reply = "Reply"
    public static let delete = "Delete"
    public static let deleteTitle = "Delete this message?"
    public static let deleteMessage = "It's removed for everyone in the cabal."
    public static let cancel = "Cancel"
    public static let composerPlaceholder = "Reply in thread"
    public static let alsoInChannel = "Also send to channel"
    public static let noReplies = "No replies yet. Start the thread."
    public static let loadFailure = "Couldn't load this thread."
    public static let headerPrefix = "replied to a thread"
    public static let snippetLength = 60

    public static func summary(replyCount: Int, lastReplyAt: Date?, now: Date, calendar: Calendar = .current)
        -> String
    {
        let count = GroupChatCopy.replies(replyCount)
        guard let lastReplyAt else { return count }
        return "\(count) · last reply \(age(of: lastReplyAt, now: now, calendar: calendar))"
    }

    public static func header(parentBody: String?) -> String {
        guard let snippet = parentBody?.trimmingCharacters(in: .whitespacesAndNewlines), !snippet.isEmpty else {
            return headerPrefix
        }
        return "\(headerPrefix): \(String(snippet.prefix(snippetLength)))"
    }

    private static func age(of date: Date, now: Date, calendar: Calendar) -> String {
        let label = RelativeTimeFormatter.label(date: date, now: now, calendar: calendar)
        if label == "now" { return "just now" }
        let isElapsed = label.last.map { "mh".contains($0) } == true && label.dropLast().allSatisfy(\.isNumber)
        return isElapsed ? "\(label) ago" : "on \(label)"
    }
}
