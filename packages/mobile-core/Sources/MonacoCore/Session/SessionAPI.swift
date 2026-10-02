import Foundation
import MonacoAPI

public struct SessionAPI: Sendable {
    private let api: APIClient
    private let submission: IdempotentSubmission

    public init(api: APIClient, submission: IdempotentSubmission = IdempotentSubmission()) {
        self.api = api
        self.submission = submission
    }

    public func openSession() async throws -> SessionProfile {
        try await api.submit(submission, payload: EmptyBody(), operation: "postAuthSession") { _, _ in
            let data = try await self.api.sessionBody { client in
                _ = try await client.postAuthSession().ok
            }
            return try SessionProfile(json: data)
        }
    }

    public func me() async throws -> SessionProfile {
        let data = try await api.sessionBody { client in
            _ = try await client.getMe().ok
        }
        return try SessionProfile(json: data)
    }
}

private struct EmptyBody: Encodable, Sendable {}
