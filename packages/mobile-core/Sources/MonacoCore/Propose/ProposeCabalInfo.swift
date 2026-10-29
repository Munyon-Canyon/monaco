import MonacoAPI

public struct ProposeCabalInfo: Equatable, Sendable {
    public let name: String
    public let voters: String
    public let votingWindow: String
    public let votesToPass: String
    public let votingWindowShort: String
    public let voterCount: Int
    public let neededToPass: Int

    public init(
        name: String, voters: String, votingWindow: String = "", votesToPass: String = "",
        votingWindowShort: String = "", voterCount: Int = 0, neededToPass: Int = 0
    ) {
        self.name = name
        self.voters = voters
        self.votingWindow = votingWindow
        self.votesToPass = votesToPass
        self.votingWindowShort = votingWindowShort
        self.voterCount = voterCount
        self.neededToPass = neededToPass
    }

    static func shortWindow(seconds: Int32) -> String {
        let hours = Int(seconds) / 3_600
        return hours >= 48 ? "\(hours / 24) d" : "\(max(hours, 1)) h"
    }

    public init(_ cabal: Components.Schemas.Cabal) {
        let voting = cabal.members.filter(\.canVote)
        name = cabal.name
        votingWindow =
            CabalProposalExpiry(rawValue: cabal.rules.proposalExpirySeconds)?.label
            ?? CabalRulesSummary.durationLabel(seconds: cabal.rules.proposalExpirySeconds)
        votesToPass = CabalThreshold(rawValue: cabal.rules.threshold)?.label ?? cabal.rules.threshold
        votingWindowShort = Self.shortWindow(seconds: cabal.rules.proposalExpirySeconds)
        voterCount = voting.count
        let unanimous = CabalThreshold(rawValue: cabal.rules.threshold) == .unanimous
        neededToPass = unanimous ? voting.count : voting.count / 2 + 1
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
