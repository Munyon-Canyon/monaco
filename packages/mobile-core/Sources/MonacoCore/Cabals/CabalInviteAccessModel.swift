import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalInviteAccessModel {
    public private(set) var standing: CabalInviteStanding?

    public let cabalID: String
    private let api: APIClient

    public init(cabalID: String, api: APIClient) {
        self.cabalID = cabalID
        self.api = api
    }

    public var canInvite: Bool {
        standing?.canInvite ?? false
    }

    public func load() async {
        let cabalID = cabalID
        do {
            let cabal = try await api.read { client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json
            }
            standing = CabalInviteStanding(cabal)
        } catch {
            return
        }
    }
}
