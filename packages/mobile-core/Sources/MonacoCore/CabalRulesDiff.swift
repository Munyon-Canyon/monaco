import Foundation
import MonacoAPI

public struct CabalSettings: Equatable, Sendable {
    public var name: String
    public var joinMode: String
    public var threshold: String
    public var proposalExpirySeconds: Int32

    public init(name: String, joinMode: String, threshold: String, proposalExpirySeconds: Int32) {
        self.name = name
        self.joinMode = joinMode
        self.threshold = threshold
        self.proposalExpirySeconds = proposalExpirySeconds
    }

    public init(_ cabal: Components.Schemas.Cabal) {
        self.init(
            name: cabal.name,
            joinMode: cabal.rules.joinMode,
            threshold: cabal.rules.threshold,
            proposalExpirySeconds: cabal.rules.proposalExpirySeconds
        )
    }
}

public enum CabalRulesDiff {
    public static func patch(
        from current: CabalSettings,
        to edited: CabalSettings
    ) -> Components.Schemas.UpdateCabalRequest {
        var body = Components.Schemas.UpdateCabalRequest()
        let name = edited.name.trimmingCharacters(in: .whitespacesAndNewlines)
        if name != current.name {
            body.name = name
        }
        if edited.joinMode != current.joinMode {
            body.joinMode = edited.joinMode
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
