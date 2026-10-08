import Foundation
import MonacoAPI

public struct CabalSettings: Equatable, Sendable {
    public var name: String
    public var joinPolicy: CabalJoinPolicy
    public var threshold: String
    public var proposalExpirySeconds: Int32
    public var voters: CabalVoterChoice

    public init(
        name: String,
        threshold: String,
        proposalExpirySeconds: Int32,
        voters: CabalVoterChoice = .everyone,
        joinPolicy: CabalJoinPolicy = .request
    ) {
        self.name = name
        self.joinPolicy = joinPolicy
        self.threshold = threshold
        self.proposalExpirySeconds = proposalExpirySeconds
        self.voters = voters
    }

    public init(_ cabal: Components.Schemas.Cabal) {
        self.init(
            name: cabal.name,
            threshold: cabal.rules.threshold,
            proposalExpirySeconds: cabal.rules.proposalExpirySeconds,
            voters: CabalVoterChoice(cabal),
            joinPolicy: CabalJoinPolicy(wire: cabal.rules.joinMode)
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
        if edited.joinPolicy != current.joinPolicy {
            body.joinMode = edited.joinPolicy.rawValue
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
