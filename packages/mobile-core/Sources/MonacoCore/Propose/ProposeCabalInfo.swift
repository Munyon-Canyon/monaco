import MonacoAPI

public struct ProposeCabalInfo: Equatable, Sendable {
    public let name: String
    public let voters: String

    public init(name: String, voters: String) {
        self.name = name
        self.voters = voters
    }

    public init(_ cabal: Components.Schemas.Cabal) {
        let voting = cabal.members.filter(\.canVote)
        name = cabal.name
        if voting.count == cabal.members.count, voting.count > 1 {
            voters = "All \(voting.count) members"
        } else {
            voters = voting.map { $0.displayName.isEmpty ? ($0.handle ?? "A member") : $0.displayName }
                .joined(separator: ", ")
        }
    }

    public static func load(api: APIClient, cabalID: String) async throws -> ProposeCabalInfo {
        let cabal = try await api.read { client in
            try await client.getCabal(path: .init(id: cabalID)).ok.body.json
        }
        return ProposeCabalInfo(cabal)
    }
}
