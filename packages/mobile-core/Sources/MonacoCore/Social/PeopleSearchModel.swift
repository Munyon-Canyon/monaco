import Foundation
import MonacoAPI
import Observation

public struct PeopleSearchToast: Equatable, Sendable {
    public let serial: Int
    public let message: String
}

@Observable
@MainActor
public final class PeopleSearchModel {
    public static let debounce: Duration = .milliseconds(300)
    public static let minimumLength = 2

    public var query = "" {
        didSet {
            guard normalized != Self.normalize(oldValue) else { return }
            schedule(after: Self.debounce)
        }
    }
    public private(set) var state: LoadState<[Components.Schemas.UserSummary]> = .idle
    public private(set) var toast: PeopleSearchToast?

    private let api: APIClient
    private var toggling: Set<String> = []
    private let clock: any Clock<Duration>
    private var generation = 0
    private var task: Task<Void, Never>?

    public init(api: APIClient, clock: any Clock<Duration>) {
        self.api = api
        self.clock = clock
    }

    public var normalized: String { Self.normalize(query) }

    public var isSearching: Bool { normalized.count >= Self.minimumLength }

    public func retry() {
        schedule(after: nil)
    }

    public func isToggling(_ userID: String) -> Bool {
        toggling.contains(userID)
    }

    public func toggleFollow(_ userID: String) async {
        guard !toggling.contains(userID), let previous = followedByMe(userID) else { return }
        toggling.insert(userID)
        defer { toggling.remove(userID) }
        set(userID, following: !previous)
        do {
            let state = try await sendFollow(userID, following: !previous)
            set(userID, following: state.following)
        } catch {
            set(userID, following: previous)
            toast = PeopleSearchToast(
                serial: (toast?.serial ?? 0) + 1, message: ToastCopy.message(for: APIError(error)))
        }
    }

    private func followedByMe(_ userID: String) -> Bool? {
        guard case .loaded(let users) = state else { return nil }
        return users.first { $0.userId == userID }?.followedByMe
    }

    private func set(_ userID: String, following: Bool) {
        guard case .loaded(var users) = state, let index = users.firstIndex(where: { $0.userId == userID }) else {
            return
        }
        users[index].followedByMe = following
        state = .loaded(users)
    }

    private func sendFollow(_ userID: String, following: Bool) async throws -> Components.Schemas.FollowState {
        let submission = IdempotentSubmission()
        let call = FollowCall(userID: userID, following: following)
        if following {
            return try await api.submit(submission, payload: call, operation: "postUserFollow") { client, key in
                try await client.postUserFollow(
                    path: .init(id: userID),
                    headers: .init(idempotencyKey: key),
                    body: .json(.init(source: "profile"))
                ).ok.body.json
            }
        }
        return try await api.submit(submission, payload: call, operation: "deleteUserFollow") { client, key in
            try await client.deleteUserFollow(
                path: .init(id: userID),
                headers: .init(idempotencyKey: key)
            ).ok.body.json
        }
    }

    private func schedule(after delay: Duration?) {
        task?.cancel()
        generation += 1
        let issued = generation
        let text = normalized
        guard text.count >= Self.minimumLength else {
            task = nil
            state = .idle
            return
        }
        if !hasRows { state = .loading }
        task = Task { [weak self, clock] in
            if let delay {
                do { try await clock.sleep(for: delay) } catch { return }
            }
            await self?.run(text, issued: issued)
        }
    }

    private func run(_ text: String, issued: Int) async {
        let result: Result<[Components.Schemas.UserSummary], APIError>
        do {
            let users = try await api.read { client in
                try await client.searchUsers(query: .init(query: text)).ok.body.json.users
            }
            result = .success(users)
        } catch {
            result = .failure(APIError(error))
        }
        guard issued == generation else { return }
        task = nil
        switch result {
        case .success(let users):
            state = .loaded(users)
        case .failure(let error) where hasRows:
            toast = PeopleSearchToast(serial: (toast?.serial ?? 0) + 1, message: ToastCopy.message(for: error))
        case .failure(let error):
            state = .failed(error)
        }
    }

    private var hasRows: Bool {
        if case .loaded(let users) = state { return !users.isEmpty }
        return false
    }

    private struct FollowCall: Encodable, Sendable {
        var userID: String
        var following: Bool
    }

    private static func normalize(_ text: String) -> String {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.hasPrefix("@") ? String(trimmed.dropFirst()) : trimmed
    }
}
