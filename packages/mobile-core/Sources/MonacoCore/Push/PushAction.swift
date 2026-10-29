import Foundation

public enum PushAction {
    public static let voteYes = "VOTE_YES"
    public static let voteNo = "VOTE_NO"
    public static let readThesis = "READ_THESIS"

    public static func route(actionIdentifier: String, payload: [String: Any]) -> PushRoute {
        guard let ids = voteIDs(payload) else { return PushRoute.parse(payload) }
        switch actionIdentifier {
        case voteYes: return .voteHold(proposalID: ids.proposalID, cabalID: ids.cabalID, pick: .yes)
        case voteNo: return .voteHold(proposalID: ids.proposalID, cabalID: ids.cabalID, pick: .no)
        case readThesis: return .proposal(proposalID: ids.proposalID, cabalID: ids.cabalID)
        default: return PushRoute.parse(payload)
        }
    }

    public static func analyticsName(actionIdentifier: String) -> String {
        switch actionIdentifier {
        case voteYes: "vote_yes"
        case voteNo: "vote_no"
        case readThesis: "read_thesis"
        default: "default"
        }
    }

    private static func voteIDs(_ payload: [String: Any]) -> (proposalID: String, cabalID: String)? {
        guard let proposalID = uuid(payload["proposal_id"]), let cabalID = uuid(payload["cabal_id"]) else { return nil }
        return (proposalID, cabalID)
    }

    private static func uuid(_ value: Any?) -> String? {
        guard let text = value as? String, let uuid = UUID(uuidString: text) else { return nil }
        return uuid.uuidString.lowercased()
    }
}
