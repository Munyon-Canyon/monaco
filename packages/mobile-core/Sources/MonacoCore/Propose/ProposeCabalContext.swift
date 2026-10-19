import MonacoAPI
import Observation

@MainActor @Observable
public final class ProposeCabalContext {
    public enum Standing: Equatable, Sendable { case unknown, member, voter, notMember }

    public private(set) var name: String?
    public private(set) var pictureURL: String?
    public private(set) var standing = Standing.unknown

    public init() {}

    public func load(api: APIClient, cabalID: String) async {
        guard
            let cabal = try? await api.read({ client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json
            })
        else { return }
        name = cabal.name
        pictureURL = cabal.pictureUrl
        guard let me = cabal.me else {
            standing = .notMember
            return
        }
        standing = me.canVote ? .voter : .member
    }
}
