import Foundation
import MonacoAPI

public struct CabalSettings: Equatable, Sendable {
    public enum Voters: Equatable, Sendable {
        case everyone
        case justMe
    }

    public var name: String
    public var joinMode: String
    public var voters: Voters
    public var threshold: String
    public var proposalExpirySeconds: Int32

    public init(name: String, joinMode: String, voters: Voters, threshold: String, proposalExpirySeconds: Int32) {
        self.name = name
        self.joinMode = joinMode
        self.voters = voters
        self.threshold = threshold
        self.proposalExpirySeconds = proposalExpirySeconds
    }

    public init(_ cabal: Components.Schemas.Cabal) {
        self.init(
            name: cabal.name,
            joinMode: cabal.rules.joinMode,
            voters: cabal.rules.voterMode == "list" ? .justMe : .everyone,
            threshold: cabal.rules.threshold,
            proposalExpirySeconds: cabal.rules.proposalExpirySeconds
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
        if edited.joinMode != current.joinMode {
            body.joinMode = edited.joinMode
        }
        if edited.voters != current.voters {
            switch edited.voters {
            case .everyone:
                body.voterMode = "all"
            case .justMe:
                body.voterMode = "list"
                body.voterIds = [creatorID]
            }
        }
        if edited.threshold != current.threshold {
            body.threshold = edited.threshold
        }
        if edited.proposalExpirySeconds != current.proposalExpirySeconds {
            body.proposalExpirySeconds = edited.proposalExpirySeconds
        }
        return body
    }
}

extension Components.Schemas.UpdateCabalRequest {
    public var isEmpty: Bool {
        self == Components.Schemas.UpdateCabalRequest()
    }
}
