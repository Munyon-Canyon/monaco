import Foundation
import MonacoAPI
import MonacoFlows
import Observation

public struct LeaveStanding: Sendable, Equatable {
    public let cabalName: String
    public let canLeave: Bool

    public init(cabalName: String, canLeave: Bool) {
        self.cabalName = cabalName
        self.canLeave = canLeave
    }

    init(_ cabal: Components.Schemas.MyCabal) {
        self.init(cabalName: cabal.name, canLeave: cabal.role != "creator" || cabal.memberCount <= 1)
    }
}

public enum LeaveOutcome: Sendable, Equatable {
    case left(cabalName: String)
    case cashOutFirst(message: String)
    case refused(message: String)

    public init(refusal error: APIError) {
        let message = ToastCopy.message(for: error)
        switch Flow04Outcome(error) {
        case .leaveHoldsShares:
            self = .cashOutFirst(message: message)
        case .leaveLastMemberPotNotEmpty, .leaveCreatorWithMembers, .notCabalMember, .unauthorized,
            .priceUnavailable, .ok, .interrupted, nil:
            self = .refused(message: message)
        }
    }
}

@Observable
@MainActor
public final class LeaveCabalModel {
    public private(set) var standing: LoadState<LeaveStanding?> = .idle
    public private(set) var isLeaving = false

    private let api: APIClient
    private let hints: any HintSource
    private let cabalID: String
    private let submission = IdempotentSubmission()
    private var generation = 0

    public init(api: APIClient, hints: any HintSource, cabalID: String) {
        self.api = api
        self.hints = hints
        self.cabalID = cabalID
    }

    public func load() async {
        generation += 1
        let mine = generation
        if case .idle = standing {
            standing = .loading
        }
        do {
            let cabals = try await api.read { client in
                try await client.getMyCabals().ok.body.json
            }
            guard mine == generation else { return }
            standing = .loaded(cabals.first { $0.id == cabalID }.map(LeaveStanding.init))
        } catch {
            guard mine == generation else { return }
            standing = .failed(APIError(error))
        }
    }

    public func observe() async {
        for await _ in hints.hints(matching: .cabal(id: cabalID, what: "members")) {
            if Task.isCancelled { return }
            await load()
        }
    }

    public func leave() async -> LeaveOutcome? {
        guard !isLeaving, case .loaded(let standing?) = standing, standing.canLeave else { return nil }
        isLeaving = true
        defer { isLeaving = false }
        let cabalID = cabalID
        do {
            _ = try await api.submit(submission, payload: cabalID, operation: "deleteCabalMemberMe") { client, key in
                try await client.deleteCabalMemberMe(
                    path: .init(id: cabalID),
                    headers: .init(idempotencyKey: key)
                ).noContent
            }
            return .left(cabalName: standing.cabalName)
        } catch {
            return LeaveOutcome(refusal: APIError(error))
        }
    }
}

#if DEBUG
extension LeaveCabalModel {
    public nonisolated static let previewCabalID = "00000000-0000-4000-8000-000000000004"

    public static func preview(answering scenario: Flow04Scenario) -> LeaveCabalModel {
        preview(transport: LeaveCabalPreviewTransport(role: "member", memberCount: 2, refusal: scenario))
    }

    public static func preview(role: String, memberCount: Int) -> LeaveCabalModel {
        preview(transport: LeaveCabalPreviewTransport(role: role, memberCount: memberCount, refusal: nil))
    }

    private static func preview(transport: some ClientTransport) -> LeaveCabalModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return LeaveCabalModel(
            api: APIClient(serverURL: serverURL, tokens: LeaveCabalPreviewTokens(), transport: transport),
            hints: LeaveCabalPreviewHints(),
            cabalID: previewCabalID
        )
    }
}

private struct LeaveCabalPreviewTransport: ClientTransport {
    let role: String
    let memberCount: Int
    let refusal: Flow04Scenario?

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        guard request.method == .delete else {
            var response = HTTPResponse(status: .ok)
            response.headerFields[.contentType] = "application/json"
            let row =
                #"{"id":"\#(LeaveCabalModel.previewCabalID)","name":"QA pot","picture_url":null,"#
                + #""role":"\#(role)","can_vote":true,"member_count":\#(memberCount),"#
                + #""joined_at":"2026-10-02T15:00:00Z","pending_request_count":0,"unread_count":0}"#
            return (response, HTTPBody("[\(row)]"))
        }
        switch refusal {
        case nil:
            return (HTTPResponse(status: .noContent), nil)
        case .unauthorized:
            return Self.problem(401, code: "unauthorized", message: "Sign in to continue.")
        case .notCabalMember:
            return Self.problem(403, code: "not_cabal_member", message: "You are not a member of this cabal.")
        case .leaveHoldsShares:
            return Self.problem(
                409, code: "leave_holds_shares", message: "Cash out your share of the pot before you leave this cabal.")
        case .leaveLastMemberPotNotEmpty:
            return Self.problem(
                409, code: "leave_last_member_pot_not_empty",
                message: "The pot still holds money, so the last member cannot leave yet.")
        case .leaveCreatorWithMembers:
            return Self.problem(
                409, code: "leave_creator_with_members",
                message: "The creator cannot leave while other members remain.")
        case .priceUnavailable:
            return Self.problem(
                503, code: "price_unavailable", message: "Prices are temporarily unavailable. Try again in a moment.")
        case .interrupted:
            throw URLError(.networkConnectionLost)
        }
    }

    private static func problem(_ status: Int, code: String, message: String) -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .init(code: status))
        response.headerFields[.contentType] = "application/problem+json"
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","#
            + #""message":"\#(message)","trace_id":"00000000000000000000000000000000","retryable":false}"#
        return (response, HTTPBody(body))
    }
}

private struct LeaveCabalPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct LeaveCabalPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { continuation in
            continuation.finish()
        }
    }
}
#endif
