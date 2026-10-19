import MonacoAPI

public struct ProposeCabalInfo: Equatable, Sendable {
    public let name: String
    public let voters: String
    public let votingWindow: String
    public let votesToPass: String

    public init(name: String, voters: String, votingWindow: String = "", votesToPass: String = "") {
        self.name = name
        self.voters = voters
        self.votingWindow = votingWindow
        self.votesToPass = votesToPass
    }

    public init(_ cabal: Components.Schemas.Cabal) {
        let voting = cabal.members.filter(\.canVote)
        name = cabal.name
        votingWindow =
            CabalProposalExpiry(rawValue: cabal.rules.proposalExpirySeconds)?.label
            ?? CabalRulesSummary.durationLabel(seconds: cabal.rules.proposalExpirySeconds)
        votesToPass = CabalThreshold(rawValue: cabal.rules.threshold)?.label ?? cabal.rules.threshold
        if voting.count == cabal.members.count, voting.count > 1 {
            voters = "All \(voting.count) members"
        } else {
            let names = voting.map { $0.displayName.isEmpty ? ($0.handle ?? "A member") : $0.displayName }
            voters =
                names.count > 3
                ? "\(names[0]), \(names[1]) and \(names.count - 2) more" : names.joined(separator: ", ")
        }
    }

    public static func load(api: APIClient, cabalID: String) async throws -> ProposeCabalInfo {
        let cabal = try await api.read { client in
            try await client.getCabal(path: .init(id: cabalID)).ok.body.json
        }
        return ProposeCabalInfo(cabal)
    }
}
