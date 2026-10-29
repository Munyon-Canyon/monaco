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
        if proposal.status == "open" { parts.append(CatchUpCopy.endsIn(proposal.expiresAt, now: now)) }
        return parts.joined(separator: " · ")
    }

    public static func closedLine(_ item: Item) -> String {
        guard let proposal = item.proposal else { return "" }
        var parts: [String] = []
        switch proposal.yourChoice {
        case .yes: parts.append("you voted Yes")
        case .no: parts.append("you voted No")
        case nil: break
        }
        if proposal.outcome == .filled, proposal.filled == true { parts.append("filled") }
        return parts.joined(separator: " · ")
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
