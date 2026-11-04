import Foundation
import MonacoAPI

public enum CatchUpDeckCopy {
    public typealias Item = Components.Schemas.CatchUpItem
    public typealias Proposal = Components.Schemas.CatchUpProposal

    public static let keep = "Keep"
    public static let clear = "Clear"
    public static let close = "Close"
    public static let upNext = "Up next"
    public static let seeAll = "See all"
    public static let reply = "Reply"
    public static let vote = "Vote"
    public static let join = "Join"
    public static let openProposal = "Open proposal"
    public static let openPost = "Open post"
    public static let openCabal = "Open cabal"

    public struct Segment: Equatable, Sendable {
        public let text: String
        public let isMention: Bool

        public init(text: String, isMention: Bool) {
            self.text = text
            self.isMention = isMention
        }
    }

    public static func proposedLine(_ proposal: Proposal, now: Date, calendar: Calendar) -> String {
        let when = CatchUpCopy.relativeTime(proposal.createdAt, now: now, calendar: calendar)
        let name = proposal.proposer.name
        return name.isEmpty ? "Proposed · \(when)" : "\(name) proposed · \(when)"
    }

    public static func statusLine(_ proposal: Proposal, now: Date) -> String {
        var parts: [String] = ["\(proposal.voted) of \(proposal.eligible) voted"]
        if proposal.replyCount > 0 {
            parts.append("\(proposal.replyCount) \(proposal.replyCount == 1 ? "reply" : "replies")")
        }
        if proposal.status == "open", let closes = closesIn(proposal.expiresAt, now: now) { parts.append(closes) }
        return parts.joined(separator: " · ")
    }

    public static func closedLine(_ item: Item) -> String {
        switch item.proposal?.yourChoice {
        case .yes: "you said yes"
        case .no: "you said no"
        case nil: ""
        }
    }

    private static func closesIn(_ expires: Date, now: Date) -> String? {
        let remaining = Int(expires.timeIntervalSince(now))
        guard remaining > 0 else { return nil }
        let days = remaining / 86400
        let hours = remaining / 3600
        if days >= 2 { return "closes in \(days)d" }
        if hours > 0 { return "closes in \(hours)h" }
        return "closes in \(max(remaining / 60, 1))m"
    }

    public static func postHead(_ post: Components.Schemas.CatchUpPost) -> String {
        post.actorName.isEmpty ? post.label : "\(post.actorName) · \(post.label)"
    }

    public static func openLabel(_ item: Item) -> String {
        switch item.kind {
        case .waitingVote, .result: openProposal
        case .reply, .mention: item.proposal == nil && item.post != nil ? openPost : openProposal
        case .invite: openCabal
        }
    }

    public static func segments(_ body: String) -> [Segment] {
        var out: [Segment] = []
        var plain = ""
        var index = body.startIndex
        while index < body.endIndex {
            let char = body[index]
            let previousIsWord = index > body.startIndex && isWord(body[body.index(before: index)])
            if char == "@", !previousIsWord {
                var end = body.index(after: index)
                while end < body.endIndex, isWord(body[end]) { end = body.index(after: end) }
                if end > body.index(after: index) {
                    if !plain.isEmpty { out.append(Segment(text: plain, isMention: false)) }
                    plain = ""
                    out.append(Segment(text: String(body[index..<end]), isMention: true))
                    index = end
                    continue
                }
            }
            plain.append(char)
            index = body.index(after: index)
        }
        if !plain.isEmpty { out.append(Segment(text: plain, isMention: false)) }
        return out
    }

    private static func isWord(_ char: Character) -> Bool {
        char.isLetter || char.isNumber || char == "_"
    }

}
