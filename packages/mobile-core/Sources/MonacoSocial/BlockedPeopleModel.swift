import MonacoAPI
import MonacoFlows
import Observation

public struct BlockedPerson: Identifiable, Equatable, Sendable {
    public let id: String
    public let handle: String
    public let displayName: String
    public let photoURL: String?

    public var title: String { displayName.isEmpty ? "Deleted account" : displayName }
    public var subtitle: String? { handle.isEmpty ? nil : "@\(handle)" }

    public init(id: String, handle: String, displayName: String, photoURL: String?) {
        self.id = id
        self.handle = handle
        self.displayName = displayName
        self.photoURL = photoURL
    }

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
    public let actions: Flow20aReportBlockModel
    private let api: APIClient

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        actions = Flow20aReportBlockModel(api: api, hints: hints)
    }

    public func unblock(userID: String) async -> Flow20aOutcome {
        guard case .loaded(let people) = state, let index = people.firstIndex(where: { $0.id == userID }) else {
            return await actions.unblock(userID: userID)
        }
        let removed = people[index]
        state = .loaded(people.filter { $0.id != userID })
        let outcome = await actions.unblock(userID: userID)
        if outcome != .ok, case .loaded(let current) = state, !current.contains(where: { $0.id == userID }) {
            var restored = current
            restored.insert(removed, at: min(index, restored.count))
            state = .loaded(restored)
        }
        return outcome
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
