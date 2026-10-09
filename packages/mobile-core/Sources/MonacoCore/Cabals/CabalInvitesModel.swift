import Foundation
import MonacoAPI
import Observation

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct ReceivedCabalInvite: Identifiable, Equatable, Sendable {
    public let id: String
    public let cabalID: String
    public let cabalName: String
    public let pictureURL: String?
    public let members: String
    public let invitedBy: String

    init(_ invite: Components.Schemas.CabalInvite) {
        id = invite.requestId
        cabalID = invite.cabal.id
        cabalName = invite.cabal.name
        pictureURL = invite.cabal.pictureUrl
        members = invite.cabal.memberCount == 1 ? "1 member" : "\(invite.cabal.memberCount) members"
        let inviter = invite.invitedBy
        let name =
            inviter.handle.map { "@\($0)" }
            ?? (inviter.displayName.isEmpty ? "Someone" : inviter.displayName)
        invitedBy = "\(name) invited you"
    }
}

@Observable
@MainActor
public final class CabalInvitesModel {
    public private(set) var state: LoadState<[ReceivedCabalInvite]> = .idle
    public private(set) var answering: Set<String> = []
    public private(set) var toast: CabalInviteToast?

    private let api: APIClient
    private let hints: any HintSource
    private let refresher: HintRefresher
    private var submissions: [String: IdempotentSubmission] = [:]
    private var generation = 0
    private var toastSerial = 0

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
        let hook = InvitesReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.load()
        }
    }

    public var invites: [ReceivedCabalInvite] {
        if case .loaded(let invites) = state { return invites }
        return []
    }

    public func load() async {
        generation += 1
        let mine = generation
        if !isLoaded {
            state = .loading
        }
        do {
            let invites = try await api.read { client in
                try await client.getMyCabalInvites().ok.body.json
            }
            guard mine == generation else { return }
            state = .loaded(invites.map(ReceivedCabalInvite.init))
        } catch {
            guard mine == generation else { return }
            if !isLoaded {
                state = .failed(APIError(error))
            }
        }
    }

    private var isLoaded: Bool {
        if case .loaded = state { return true }
        return false
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: "cabal_invites")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func accept(_ invite: ReceivedCabalInvite) async -> Bool {
        await answer(invite, decision: .approve, success: CabalEntry.joinedToast)
    }

    public func decline(_ invite: ReceivedCabalInvite) async {
        _ = await answer(invite, decision: .deny, success: "Invite declined.")
    }

    private func answer(
        _ invite: ReceivedCabalInvite,
        decision: Components.Schemas.AccessDecisionRequest.DecisionPayload,
        success: String
    ) async -> Bool {
        guard !answering.contains(invite.id) else { return false }
        answering.insert(invite.id)
        defer { answering.remove(invite.id) }
        let cabalID = invite.cabalID
        let requestID = invite.id
        let body = Components.Schemas.AccessDecisionRequest(decision: decision)
        let submission = submissions[requestID] ?? IdempotentSubmission()
        submissions[requestID] = submission
        do {
            _ = try await api.submit(
                submission,
                payload: DecisionPayload(cabalID: cabalID, requestID: requestID, decision: decision.rawValue),
                operation: "postCabalAccessDecision"
            ) { client, key in
                try await client.postCabalAccessDecision(
                    path: .init(id: cabalID, requestId: requestID),
                    headers: .init(idempotencyKey: key),
                    body: .json(body)
                ).ok.body.json
            }
            drop(invite)
            show(success, success: true)
            await load()
            return true
        } catch {
            let failure = APIError(error)
            if Self.isGone(failure) {
                drop(invite)
            }
            show(ToastCopy.message(for: failure), success: false)
            return false
        }
    }

    private static func isGone(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error, let outcome = Flow03Outcome(code: problem.code.wire) else {
            return false
        }
        switch outcome {
        case .inviteExpired, .accessRequestNotPending:
            return true
        case .ok, .invalidInput, .unauthorized, .cabalNotFound, .cabalBanned, .alreadyMember, .joinNeedsRequest,
            .requestNotNeeded, .requestPending, .notCabalCreator, .cannotRevokeAccess, .userNotFound, .notCabalMember:
            return false
        }
    }

    private func drop(_ invite: ReceivedCabalInvite) {
        guard case .loaded(let invites) = state else { return }
        state = .loaded(invites.filter { $0.id != invite.id })
    }

    private func show(_ message: String, success: Bool) {
        toastSerial += 1
        toast = CabalInviteToast(serial: toastSerial, message: message, isSuccess: success)
    }
}

private struct DecisionPayload: Encodable, Sendable {
    let cabalID: String
    let requestID: String
    let decision: String
}

private final class InvitesReloadHook {
    var run: (@MainActor () async -> Void)?
}

#if DEBUG
extension CabalInvitesModel {
    public static func preview(_ invites: [Components.Schemas.CabalInvite] = Components.Schemas.CabalInvite.samples)
        -> CabalInvitesModel
    {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return CabalInvitesModel(
            api: APIClient(
                serverURL: serverURL, tokens: CabalInvitesPreviewTokens(),
                transport: CabalInvitesPreviewTransport(invites: invites)
            ),
            hints: CabalInvitesPreviewHints()
        )
    }
}

private actor CabalInvitesPreviewTransport: ClientTransport {
    private var invites: [Components.Schemas.CabalInvite]

    init(invites: [Components.Schemas.CabalInvite]) {
        self.invites = invites
    }

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        guard operationID == "postCabalAccessDecision" else {
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            return (response, HTTPBody(try encoder.encode(invites)))
        }
        let requestID = request.path?.split(separator: "/").dropLast().last.map(String.init) ?? ""
        invites.removeAll { $0.requestId == requestID }
        return (response, HTTPBody(#"{"id":"\#(requestID)","direction":"invite","status":"approved"}"#))
    }
}

private struct CabalInvitesPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct CabalInvitesPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
