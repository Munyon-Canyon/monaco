import Foundation
import MonacoAPI

public struct SessionAPI: Sendable {
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func openSession() async throws -> SessionProfile {
        let data = try await api.sessionBody { client in
            _ = try await client.postAuthSession().ok
        }
        return try SessionProfile(json: data)
    }

    public func me() async throws -> SessionProfile {
        let data = try await api.sessionBody { client in
            _ = try await client.getMe().ok
        }
        return try SessionProfile(json: data)
    }
}
