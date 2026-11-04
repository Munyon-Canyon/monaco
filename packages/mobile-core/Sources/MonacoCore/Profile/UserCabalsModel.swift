import MonacoAPI
import Observation

public struct UserCabalRow: Equatable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let pictureURL: String?
    public let subtitle: String
    public let ownership: String?
    public let isShared: Bool

    public init(_ cabal: Components.Schemas.UserCabal) {
        let role = cabal.canVote && cabal.role == .member ? "voter" : cabal.role.rawValue
        id = cabal.cabal.id
        name = cabal.cabal.name
        pictureURL = cabal.cabal.pictureUrl
        subtitle = cabal.shared ? "You are both in \u{00B7} \(role)" : "\(cabal.memberCount) members \u{00B7} \(role)"
        ownership = cabal.ownershipBps.map(ProfileCabalRows.ownership(bps:))
        isShared = cabal.shared
    }
}

public struct SharedCabalsLine: Equatable, Sendable {
    public struct Mark: Equatable, Sendable, Identifiable {
        public let id: String
        public let name: String
        public let pictureURL: String?
    }

    public let names: [String]
    public let moreCount: Int
    public let text: String
    public let marks: [Mark]

    init(_ shared: [UserCabalRow]) {
        names = shared.prefix(2).map(\.name)
        moreCount = shared.count - names.count
        marks = shared.prefix(2).map { Mark(id: $0.id, name: $0.name, pictureURL: $0.pictureURL) }
        switch shared.count {
        case 1: text = "Both in \(names[0])"
        case 2: text = "Both in \(names[0]) and \(names[1])"
        default: text = "In \(names[0]), \(names[1]) and \(moreCount) more"
        }
    }
}

@Observable
@MainActor
public final class UserCabalsModel {
    public let userID: String
    public private(set) var state: LoadState<[UserCabalRow]> = .idle

    private let viewerID: String?
    private let api: APIClient

    public init(userID: String, viewerID: String? = nil, api: APIClient) {
        self.userID = userID
        self.viewerID = viewerID
        self.api = api
    }

    public var rows: [UserCabalRow] {
        if case .loaded(let rows) = state { return rows }
        return []
    }

    public var sharedRows: [UserCabalRow] {
        rows.filter(\.isShared)
    }

    public var sharedLine: SharedCabalsLine? {
        let shared = sharedRows
        guard viewerID != userID, !shared.isEmpty else { return nil }
        return SharedCabalsLine(shared)
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
