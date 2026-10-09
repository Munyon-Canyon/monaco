import MonacoAPI
import Observation

public struct BlockedPerson: Identifiable, Equatable, Sendable {
    public let id: String
    public let handle: String
    public let displayName: String
    public let photoURL: String?

    public var title: String { displayName.isEmpty ? "Deleted account" : displayName }
    public var subtitle: String? { handle.isEmpty ? nil : "@\(handle)" }

    init(_ user: Components.Schemas.BlockedUser) {
        id = user.userId
        handle = user.handle
        displayName = user.displayName
        photoURL = user.photoUrl
    }
}

@Observable
@MainActor
public final class BlockedPeopleModel {
    public private(set) var state: LoadState<[BlockedPerson]> = .idle
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func load() async {
        if case .loaded = state {} else { state = .loading }
        do {
            let page = try await api.read { try await $0.getMeBlocks().ok.body.json }
            state = .loaded(page.users.map(BlockedPerson.init))
        } catch {
            if Task.isCancelled { return }
            if case .loaded = state { return }
            state = .failed(APIError(error))
        }
    }
}
