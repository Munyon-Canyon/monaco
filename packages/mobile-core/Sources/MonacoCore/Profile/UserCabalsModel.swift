import MonacoAPI
import Observation

public struct UserCabalRow: Equatable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let pictureURL: String?
    public let subtitle: String
    public let ownership: String?

    public init(_ cabal: Components.Schemas.UserCabal) {
        let role = cabal.canVote && cabal.role == .member ? "voter" : cabal.role.rawValue
        id = cabal.cabal.id
        name = cabal.cabal.name
        pictureURL = cabal.cabal.pictureUrl
        subtitle = cabal.shared ? "You are both in \u{00B7} \(role)" : "\(cabal.memberCount) members \u{00B7} \(role)"
        ownership = cabal.ownershipBps.map(ProfileCabalRows.ownership(bps:))
    }
}

@Observable
@MainActor
public final class UserCabalsModel {
    public let userID: String
    public private(set) var state: LoadState<[UserCabalRow]> = .idle

    private let api: APIClient

    public init(userID: String, api: APIClient) {
        self.userID = userID
        self.api = api
    }

    public var rows: [UserCabalRow] {
        if case .loaded(let rows) = state { return rows }
        return []
    }

    public func load() async {
        if case .loaded = state {
        } else {
            state = .loading
        }
        do {
            let cabals = try await api.read { [userID] client in
                try await client.getUserCabals(path: .init(id: userID)).ok.body.json
            }
            state = .loaded(cabals.cabals.map(UserCabalRow.init))
        } catch {
            if case .loaded = state { return }
            state = .failed(APIError(error))
        }
    }
}
