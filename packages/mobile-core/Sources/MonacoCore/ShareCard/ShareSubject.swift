import Foundation

public enum ShareSubject: Hashable, Sendable {
    case position(cabalID: String, symbol: String)
    case passedProposal(id: String)
    case openVote(id: String)
    case stake(cabalID: String)
    case portfolio
    case ledger(week: String)
}
