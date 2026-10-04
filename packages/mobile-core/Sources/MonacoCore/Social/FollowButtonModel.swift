import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class FollowButtonModel {
    public private(set) var following = false
    public private(set) var isToggling = false
    public private(set) var unavailable = false
    public private(set) var failureTick = 0
    public private(set) var lastError: APIError?

    public let userID: String
    private let api: APIClient

    public init(userID: String, api: APIClient) {
        self.userID = userID
        self.api = api
    }

    public func toggle() async {
        guard !isToggling, !unavailable else { return }
        isToggling = true
        defer { isToggling = false }
        let wasFollowing = following
        following = !wasFollowing
        do {
            following = try await send(follow: !wasFollowing).following
        } catch {
            following = wasFollowing
            let failure = APIError(error)
            if Self.meansUnavailable(failure) {
                unavailable = true
            } else {
                lastError = failure
                failureTick += 1
            }
        }
    }

    private func send(follow: Bool) async throws -> Components.Schemas.FollowState {
        let userID = userID
        let submission = IdempotentSubmission()
        if follow {
            return try await api.submit(submission, payload: userID, operation: "postUserFollow") { client, key in
                try await client.postUserFollow(
                    path: .init(id: userID),
                    headers: .init(idempotencyKey: key),
                    body: .json(.init(source: "profile"))
                ).ok.body.json
            }
        }
        return try await api.submit(submission, payload: userID, operation: "deleteUserFollow") { client, key in
            try await client.deleteUserFollow(
                path: .init(id: userID),
                headers: .init(idempotencyKey: key)
            ).ok.body.json
        }
    }

    private static func meansUnavailable(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error else { return false }
        return ["user_not_found", "user_banned"].contains(problem.code.wire)
    }
}
