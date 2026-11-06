import Foundation

public enum ChatThreadCopy {
    public static let title = "Thread"
    public static let reply = "Reply"
    public static let delete = "Delete"
    public static let deleteTitle = "Delete this message?"
    public static let deleteMessage = "It's removed for everyone in the cabal."
    public static let cancel = "Cancel"
    public static let composerPlaceholder = "Reply in thread"
    public static let alsoInChannel = "Also send to chat"
    public static let parentDeleted = "This message was deleted."
    public static let noReplies = "No replies yet. Start the thread."
    public static let loadFailure = "Couldn't load this thread."
    public static let headerPrefix = "Replied to a thread"
    public static let snippetLength = 60

    public static func header(parentBody: String?) -> String {
        guard let snippet = parentBody?.trimmingCharacters(in: .whitespacesAndNewlines), !snippet.isEmpty else {
            return headerPrefix
        }
        let cut = snippet.count > snippetLength
        return "\(headerPrefix): \(String(snippet.prefix(snippetLength)))\(cut ? "…" : "")"
    }

    public static let proposalHeaderFallback = "replied to a proposal"
    public static let maxFaces = 3

    public static func proposalHeader(headline: String?) -> String {
        guard let headline = headline?.trimmingCharacters(in: .whitespacesAndNewlines), !headline.isEmpty else {
            return proposalHeaderFallback
        }
        return "replied to \(headline)"
    }

    public static func lineAge(of date: Date?, now: Date, calendar: Calendar = .current) -> String? {
        date.map { RelativeTimeFormatter.label(date: $0, now: now, calendar: calendar) }
    }

    public static func lineSpoken(
        replyCount: Int, lastReplyAt: Date?, names: [String], now: Date, calendar: Calendar = .current
    ) -> String {
        var parts = [GroupChatCopy.replies(replyCount)]
        if let lastReplyAt { parts.append("last \(spokenAge(of: lastReplyAt, now: now, calendar: calendar))") }
        let who = spokenNames(names)
        if !who.isEmpty { parts.append("from \(who)") }
        return parts.joined(separator: ", ") + ". Opens the thread."
    }

    public static func spokenNames(_ names: [String]) -> String {
        let named = names.filter { !$0.isEmpty }
        guard let last = named.last else { return "" }
        guard named.count > 1 else { return last }
        return named.dropLast().joined(separator: ", ") + " and " + last
    }

    static func spokenAge(of date: Date, now: Date, calendar: Calendar) -> String {
        let label = RelativeTimeFormatter.label(date: date, now: now, calendar: calendar)
        if label == "now" { return "just now" }
        guard let unit = label.last, "mh".contains(unit), let value = Int(label.dropLast()) else {
            return "on \(label)"
        }
        let noun = unit == "m" ? "minute" : "hour"
        return "\(value) \(noun)\(value == 1 ? "" : "s") ago"
    }
}
