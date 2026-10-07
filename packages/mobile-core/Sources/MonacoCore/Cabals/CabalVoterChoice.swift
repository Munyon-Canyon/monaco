import Foundation
import MonacoAPI

public enum CabalVoterChoice: Equatable, Sendable {
    case everyone
    case list(Set<String>)

    public init(_ cabal: Components.Schemas.Cabal) {
        if cabal.rules.voterMode == CabalVoterMode.picked.rawValue {
            self = .list(Set(cabal.members.filter(\.canVote).map(\.userId)))
        } else {
            self = .everyone
        }
    }

    public func patch(creatorID: String) -> Components.Schemas.UpdateCabalRequest {
        switch self {
        case .everyone:
            return .init(voterMode: CabalVoterMode.everyone.rawValue)
        case .list(let ids):
            let others = ids.subtracting([creatorID]).sorted()
            return .init(voterMode: CabalVoterMode.picked.rawValue, voterIds: [creatorID] + others)
        }
    }
}
