import Foundation
import MonacoAPI

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public enum CabalEntry: Equatable, Sendable {
    case requested
    case alreadyMember
    case requestPending
    case refused(APIError)

    public static let joinedToast = "You're in."
    public static let requestedToast = "Request sent. You'll be in once the creator says yes."

    public var isMember: Bool {
        switch self {
        case .alreadyMember: true
        case .requested, .requestPending, .refused: false
        }
    }
}

extension APIClient {
    public func enterCabal(_ cabalID: String, submission: IdempotentSubmission) async -> CabalEntry {
        do {
            try await requestAccess(cabalID, submission: submission)
            return .requested
        } catch {
            let failure = APIError(error)
            switch Self.flow03Outcome(failure) {
            case .alreadyMember: return .alreadyMember
            case .requestPending: return .requestPending
            default: return .refused(failure)
            }
        }
    }

    public func revokeAccessRequest(
        cabalID: String,
        requestID: String,
        submission: IdempotentSubmission
    ) async throws {
        _ = try await submit(
            submission,
            payload: AccessRequestPayload(cabalID: cabalID, requestID: requestID),
            operation: "deleteCabalAccessRequest"
        ) { client, key in
            try await client.deleteCabalAccessRequest(
                path: .init(id: cabalID, requestId: requestID),
                headers: .init(idempotencyKey: key)
            ).ok.body.json
        }
    }

    public func decideAccessRequest(
        cabalID: String,
        requestID: String,
        approve: Bool,
        submission: IdempotentSubmission
    ) async throws {
        let decision: Components.Schemas.AccessDecisionRequest.DecisionPayload = approve ? .approve : .deny
        _ = try await submit(
            submission,
            payload: DecisionPayload(cabalID: cabalID, requestID: requestID, decision: decision.rawValue),
            operation: "postCabalAccessDecision"
        ) { client, key in
            try await client.postCabalAccessDecision(
                path: .init(id: cabalID, requestId: requestID),
                headers: .init(idempotencyKey: key),
                body: .json(.init(decision: decision))
            ).ok.body.json
        }
    }

    static func flow03Outcome(_ error: APIError) -> Flow03Outcome? {
        guard case .problem(let problem) = error else { return nil }
        return Flow03Outcome(code: problem.code.wire)
    }

    private func requestAccess(_ cabalID: String, submission: IdempotentSubmission) async throws {
        _ = try await submit(
            submission, payload: CabalPayload(cabalID: cabalID), operation: "postCabalAccessRequest"
        ) { client, key in
            try await client.postCabalAccessRequest(path: .init(id: cabalID), headers: .init(idempotencyKey: key))
                .created.body.json
        }
    }
}

private struct CabalPayload: Encodable, Sendable {
    let cabalID: String
}

private struct AccessRequestPayload: Encodable, Sendable {
    let cabalID: String
    let requestID: String
}

private struct DecisionPayload: Encodable, Sendable {
    let cabalID: String
    let requestID: String
    let decision: String
}
