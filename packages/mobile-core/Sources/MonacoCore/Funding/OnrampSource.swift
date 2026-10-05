import Foundation
import MonacoAPI

public struct OnrampSession: Equatable, Sendable {
    public let sessionID: String
    public let url: URL
    public let expiresAt: Date

    public init(sessionID: String, url: URL, expiresAt: Date) {
        self.sessionID = sessionID
        self.url = url
        self.expiresAt = expiresAt
    }
}

public enum OnrampStatus: String, Equatable, Sendable {
    case created, opened, confirmed, submitted, cancelled, failed, expired
}

public struct OnrampSource: Sendable {
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func createSession(
        suggestedMicros: Int64?, cabalID: String?, submission: IdempotentSubmission
    ) async throws -> OnrampSession {
        let body = Components.Schemas.CreateOnrampSessionRequest(
            suggestedAmountMicros: suggestedMicros.flatMap { $0 > 0 ? String($0) : nil },
            cabalId: cabalID
        )
        let created = try await api.submit(submission, payload: body, operation: "createOnrampSession") {
            client, key in
            try await client.createOnrampSession(headers: .init(idempotencyKey: key), body: .json(body))
                .created.body.json
        }
        guard let url = URL(string: created.url) else { throw APIError.decoding("a fund page URL: \(created.url)") }
        return OnrampSession(sessionID: created.sessionId, url: url, expiresAt: created.expiresAt)
    }

    public func session(id: String) async throws -> OnrampStatus {
        let wire = try await api.read { try await $0.getOnrampSession(path: .init(id: id)).ok.body.json }
        guard let status = OnrampStatus(rawValue: wire.status.rawValue) else {
            throw APIError.decoding("an onramp status: \(wire.status.rawValue)")
        }
        return status
    }
}
