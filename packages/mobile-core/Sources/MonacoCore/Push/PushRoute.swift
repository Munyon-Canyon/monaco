import Foundation

public enum PushRoute: Equatable, Sendable {
    case userProfile(userID: String)
    case chat(cabalID: String)
    case transaction(txnID: String, cabalID: String)
    case proposal(proposalID: String, cabalID: String)
    case feedItem(feedItemID: String)
    case cabal(cabalID: String)
    case home

    public static func parse(_ userInfo: [String: Any]) -> PushRoute {
        let payload = Payload(values: userInfo)
        return payload.profile
            ?? payload.chat
            ?? payload.transaction
            ?? payload.proposal
            ?? payload.feedItem
            ?? payload.cabal
            ?? .home
    }
}

private enum Key: String {
    case kind
    case userID = "user_id"
    case cabalID = "cabal_id"
    case proposalID = "proposal_id"
    case txnID = "txn_id"
    case feedItemID = "feed_item_id"
}

private enum Kind: String {
    case newFollower = "new_follower"
    case chatMention = "chat_mention"
    case chatThreadReply = "chat_thread_reply"
    case commentReply = "comment_reply"
}

private struct Payload {
    let values: [String: Any]

    var kind: Kind? {
        (values[Key.kind.rawValue] as? String).flatMap(Kind.init(rawValue:))
    }

    func id(_ key: Key) -> String? {
        guard let text = values[key.rawValue] as? String, let uuid = UUID(uuidString: text) else { return nil }
        return uuid.uuidString.lowercased()
    }

    var profile: PushRoute? {
        guard kind == .newFollower, let userID = id(.userID) else { return nil }
        return .userProfile(userID: userID)
    }

    var chat: PushRoute? {
        guard kind == .chatMention || kind == .chatThreadReply, let cabalID = id(.cabalID) else { return nil }
        return .chat(cabalID: cabalID)
    }

    var transaction: PushRoute? {
        guard let txnID = id(.txnID), let cabalID = id(.cabalID) else { return nil }
        return .transaction(txnID: txnID, cabalID: cabalID)
    }

    var proposal: PushRoute? {
        guard let proposalID = id(.proposalID), let cabalID = id(.cabalID) else { return nil }
        return .proposal(proposalID: proposalID, cabalID: cabalID)
    }

    var feedItem: PushRoute? {
        guard kind == .commentReply, id(.proposalID) == nil, let feedItemID = id(.feedItemID) else { return nil }
        return .feedItem(feedItemID: feedItemID)
    }

    var cabal: PushRoute? {
        let others: [Key] = [.userID, .proposalID, .txnID, .feedItemID]
        guard others.allSatisfy({ id($0) == nil }), let cabalID = id(.cabalID) else { return nil }
        return .cabal(cabalID: cabalID)
    }
}
