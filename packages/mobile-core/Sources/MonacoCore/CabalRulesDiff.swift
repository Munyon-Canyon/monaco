import Foundation
import MonacoAPI

public struct CabalSettings: Equatable, Sendable {
    public var name: String
    public var threshold: String
    public var proposalExpirySeconds: Int32
    public var voters: CabalVoterChoice

    public init(
        name: String,
        threshold: String,
        proposalExpirySeconds: Int32,
        voters: CabalVoterChoice = .everyone
    ) {
        self.name = name
        self.threshold = threshold
        self.proposalExpirySeconds = proposalExpirySeconds
        self.voters = voters
    }

    public init(_ cabal: Components.Schemas.Cabal) {
        self.init(
            name: cabal.name,
            threshold: cabal.rules.threshold,
            proposalExpirySeconds: cabal.rules.proposalExpirySeconds,
            voters: CabalVoterChoice(cabal)
        )
    }
}

public enum CabalRulesDiff {
    public static func patch(
        from current: CabalSettings,
        to edited: CabalSettings,
        creatorID: String
    ) -> Components.Schemas.UpdateCabalRequest {
        var body = Components.Schemas.UpdateCabalRequest()
        let name = edited.name.trimmingCharacters(in: .whitespacesAndNewlines)
        if name != current.name {
            body.name = name
        }
        if edited.threshold != current.threshold {
            body.threshold = edited.threshold
        }
        if edited.proposalExpirySeconds != current.proposalExpirySeconds {
            body.proposalExpirySeconds = edited.proposalExpirySeconds
        }
        let voters = edited.voters.patch(creatorID: creatorID)
        if voters != current.voters.patch(creatorID: creatorID) {
            body.voterMode = voters.voterMode
            body.voterIds = voters.voterIds
        }
        return body
    }
}

extension Components.Schemas.UpdateCabalRequest {
    public var isEmpty: Bool {
        self == Components.Schemas.UpdateCabalRequest()
    }
}
