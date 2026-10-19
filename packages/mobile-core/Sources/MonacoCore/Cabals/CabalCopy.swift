import Foundation
import MonacoAPI

public enum CabalCopy {
    public static func memberCount(_ count: Int32) -> String {
        count == 1 ? "1 member" : "\(count) members"
    }

    public static func inviteShareText(code: String) -> String {
        "Join my cabal on Monaco with code \(code)"
    }

    public static func requestCount(_ count: Int32) -> String {
        count == 1 ? "1 request to join" : "\(count) requests to join"
    }

    public static func requestBadge(_ cabal: Components.Schemas.MyCabal) -> Int32? {
        cabal.role == "creator" && cabal.pendingRequestCount > 0 ? cabal.pendingRequestCount : nil
    }

    public static let chatUnreadAction = "Chat, unread messages"

    public static func unreadBadge(_ count: Int) -> String? {
        switch count {
        case ..<1: nil
        case 1...99: "\(count)"
        default: "99+"
        }
    }

    public static func unreadLabel(_ count: Int) -> String {
        switch count {
        case 1: "1 unread message"
        case 2...99: "\(count) unread messages"
        default: "More than 99 unread messages"
        }
    }

    public static func memberName(_ member: Components.Schemas.CabalMember) -> String {
        let name = member.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        if !name.isEmpty { return name }
        if let handle = member.handle, !handle.isEmpty { return "@\(handle)" }
        return "Member"
    }

    public static func roleLine(isCreator: Bool) -> String {
        isCreator ? "You’re an administrator" : "You’re a member"
    }

    public static func votersSummary(members: Int32, eligible: Int) -> String {
        "\(memberCount(members)) · \(eligible) eligible \(eligible == 1 ? "voter" : "voters")"
    }

    public static func chatFooter(cabalName: String, unread: Int) -> String {
        guard unread > 0 else { return cabalName }
        return "\(cabalName) · \(unread > 99 ? "99+" : "\(unread)") new \(unread == 1 ? "message" : "messages")"
    }

    public static func requestsLine(_ count: Int) -> String {
        count == 1 ? "1 request to review" : "\(count) requests to review"
    }

    public static let loadFailedTitle = "Couldn’t load this view"
    public static let loadFailedBody =
        "Your existing information hasn’t been changed. Try again when your connection is available."
    public static let depositLabel = "Deposit"
    public static let noShareNotice = "You’re a member. Your share is $0.00 until you deposit."
    public static let onlyVotersCaption = "Only voters can propose"
    public static let cashOutUnavailable = "Cash out unavailable: no funded share"
    public static let returnsOpenSoon = "Returns open soon"
    public static let cabalLinksOpenSoon = "Cabal links open soon"
    public static let linkCopied = "Link copied."
    public static let votersSaved = "Voters saved."
    public static let noMessagesTitle = "No messages yet"
    public static let noMessagesBody = "Say hello to your cabal."
    public static let noProposalsLine = "No proposals need your attention."
    public static let proposalHistoryHelper = "Closed decisions remain in proposal history."

    public static func eligibleVoterCount(_ cabal: Components.Schemas.Cabal) -> Int {
        cabal.rules.voterMode == CabalVoterMode.picked.rawValue
            ? cabal.members.filter(\.canVote).count : cabal.members.count
    }

    public static func whoVotes(_ cabal: Components.Schemas.Cabal) -> String {
        cabal.rules.voterMode == CabalVoterMode.picked.rawValue ? "People I pick" : "Everyone"
    }

    public static func thresholdLine(_ cabal: Components.Schemas.Cabal) -> String {
        let eligible = eligibleVoterCount(cabal)
        let needed = cabal.rules.threshold == CabalThreshold.unanimous.rawValue ? eligible : eligible / 2 + 1
        return "\(needed) of \(eligible) eligible \(eligible == 1 ? "voter" : "voters")"
    }
}
