import MonacoAPI

public struct ProposeCabalInfo: Equatable, Sendable {
    public let name: String
    public let voters: String
    public let votingWindow: String
    public let votesToPass: String
    public let votingWindowShort: String
    public let voterCount: Int
    public let neededToPass: Int
    public let faces: [String: Face]

    public struct Face: Equatable, Sendable {
        public let name: String
        public let photoURL: String?

        public init(name: String, photoURL: String? = nil) {
            self.name = name
            self.photoURL = photoURL
        }
    }

    public init(
        name: String, voters: String, votingWindow: String = "", votesToPass: String = "",
        votingWindowShort: String = "", voterCount: Int = 0, neededToPass: Int = 0,
        faces: [String: Face] = [:]
    ) {
        self.name = name
        self.voters = voters
        self.votingWindow = votingWindow
        self.votesToPass = votesToPass
        self.votingWindowShort = votingWindowShort
        self.voterCount = voterCount
        self.neededToPass = neededToPass
        self.faces = faces
    }

    static func shortWindow(seconds: Int32) -> String {
        let hours = Int(seconds) / 3_600
        if hours >= 48 { return "\(hours / 24) days" }
        return hours > 1 ? "\(hours) hours" : "1 hour"
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
        faces = Dictionary(
            cabal.members.map { ($0.userId, Face(name: $0.displayName, photoURL: $0.photoUrl)) },
            uniquingKeysWith: { first, _ in first })
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
