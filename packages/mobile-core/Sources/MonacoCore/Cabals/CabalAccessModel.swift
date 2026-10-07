import Foundation
import MonacoAPI
import Observation

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct CabalPendingRequest: Identifiable, Equatable, Sendable {
    public let id: String
    public let userID: String
    public let name: String
    public let photoURL: String?

    init(_ request: Components.Schemas.CabalAccessRequest) {
        id = request.id
        userID = request.user.userId
        photoURL = request.user.photoUrl
        let display = request.user.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        name = !display.isEmpty ? display : request.user.handle.map { "@\($0)" } ?? "Someone"
    }
}

public enum CabalAccessStanding: Equatable, Sendable {
    case hidden
    case join(CabalInviteStanding.JoinMode)
    case requested(requestID: String)
    case pending([CabalPendingRequest])

    public static func heading(count: Int) -> String {
        count == 1 ? "1 person wants to join" : "\(count) people want to join"
    }
}

@Observable
@MainActor
public final class CabalAccessModel {
    public static let approvedToast = "Approved."
    public static let deniedToast = "Denied."
    public static let approvedWithoutVoteToast = "Approved. Add them as a voter in Cabal settings."

    public private(set) var standing: CabalAccessStanding = .hidden
    public private(set) var isBusy = false
    public private(set) var deciding: Set<String> = []
    public private(set) var toast: CabalInviteToast?
    public private(set) var picksVoters = false
    public private(set) var membershipChanges = 0

    public let cabalID: String
    private let api: APIClient
    private let hints: any HintSource
    private let refresher: HintRefresher
    private let entry = IdempotentSubmission()
    private let cancellation = IdempotentSubmission()
    private var decisions: [String: IdempotentSubmission] = [:]
    private var grants: [String: IdempotentSubmission] = [:]
    private var generation = 0
    private var toastSerial = 0

    public init(cabalID: String, api: APIClient, hints: any HintSource) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
        let hook = AccessReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.load()
        }
    }

    public func load() async {
        generation += 1
        let mine = generation
        let cabalID = cabalID
        do {
            let cabal = try await api.read { client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json
            }
            let next = try await standing(for: cabal)
            guard mine == generation else { return }
            standing = next
            picksVoters = cabal.rules.voterMode == CabalVoterMode.picked.rawValue
        } catch {
            return
        }
    }

    private func standing(for cabal: Components.Schemas.Cabal) async throws -> CabalAccessStanding {
        let mode = CabalInviteStanding.JoinMode(wire: cabal.rules.joinMode)
        guard let me = cabal.me else {
            guard let request = cabal.myAccessRequest, request.status == "pending" else { return .join(mode) }
            return request.direction == "request" ? .requested(requestID: request.id) : .hidden
        }
        guard me.role == "creator", mode == .request else { return .hidden }
        let cabalID = cabalID
        let requests = try await api.read { client in
            try await client.getCabalAccessRequests(path: .init(id: cabalID)).ok.body.json
        }
        return requests.isEmpty ? .hidden : .pending(requests.map(CabalPendingRequest.init))
    }

    public func observe() async {
        await refresher.observe([
            hints.hints(matching: .cabal(id: cabalID, what: "access_requests")),
            hints.hints(matching: .cabal(id: cabalID, what: "members")),
            hints.hints(matching: .user(what: "cabal_access")),
        ])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func enter() async {
        guard case .join(let mode) = standing, !isBusy else { return }
        isBusy = true
        defer { isBusy = false }
        switch await api.enterCabal(cabalID, mode: mode, submission: entry) {
        case .joined:
            show(CabalEntry.joinedToast, success: true)
            membershipChanges += 1
        case .requested: show(CabalEntry.requestedToast, success: true)
        case .alreadyMember: membershipChanges += 1
        case .requestPending: break
        case .refused(let error):
            show(ToastCopy.message(for: error), success: false)
            return
        }
        await load()
    }

    public func cancelRequest() async {
        guard case .requested(let requestID) = standing, !isBusy else { return }
        isBusy = true
        defer { isBusy = false }
        do {
            try await api.revokeAccessRequest(cabalID: cabalID, requestID: requestID, submission: cancellation)
        } catch {
            let failure = APIError(error)
            if APIClient.flow03Outcome(failure) != .accessRequestNotPending {
                show(ToastCopy.message(for: failure), success: false)
                return
            }
        }
        await load()
    }

    public func decide(_ request: CabalPendingRequest, approve: Bool, canVote: Bool = false) async {
        guard !deciding.contains(request.id) else { return }
        deciding.insert(request.id)
        defer { deciding.remove(request.id) }
        let submission = decisions[request.id] ?? IdempotentSubmission()
        decisions[request.id] = submission
        do {
            try await api.decideAccessRequest(
                cabalID: cabalID, requestID: request.id, approve: approve, submission: submission)
            drop(request)
            if approve { membershipChanges += 1 }
            if approve, canVote, picksVoters, !(await grantVote(to: request)) {
                show(Self.approvedWithoutVoteToast, success: false)
            } else {
                show(approve ? Self.approvedToast : Self.deniedToast, success: true)
            }
        } catch {
            let failure = APIError(error)
            if APIClient.flow03Outcome(failure) == .accessRequestNotPending {
                drop(request)
            }
            show(ToastCopy.message(for: failure), success: false)
        }
        await load()
    }

    private func grantVote(to request: CabalPendingRequest) async -> Bool {
        let submission = grants[request.id] ?? IdempotentSubmission()
        grants[request.id] = submission
        let cabalID = cabalID
        do {
            let cabal = try await api.read { client in
                try await client.getCabal(path: .init(id: cabalID)).ok.body.json
            }
            guard case .list(var voters) = CabalVoterChoice(cabal) else { return true }
            voters.insert(request.userID)
            let body = CabalVoterChoice.list(voters).patch(creatorID: cabal.creator.userId)
            _ = try await api.submit(submission, payload: body, operation: "patchCabal") { client, key in
                try await client.patchCabal(
                    path: .init(id: cabalID),
                    headers: .init(idempotencyKey: key),
                    body: .json(body)
                ).ok.body.json
            }
            return true
        } catch {
            return false
        }
    }

    private func drop(_ request: CabalPendingRequest) {
        guard case .pending(let requests) = standing else { return }
        let rest = requests.filter { $0.id != request.id }
        standing = rest.isEmpty ? .hidden : .pending(rest)
    }

    private func show(_ message: String, success: Bool) {
        toastSerial += 1
        toast = CabalInviteToast(serial: toastSerial, message: message, isSuccess: success)
    }
}

private final class AccessReloadHook {
    var run: (@MainActor () async -> Void)?
}

#if DEBUG
extension CabalAccessModel {
    public static func preview(
        cabal: Components.Schemas.Cabal,
        requests: [Components.Schemas.CabalAccessRequest] = Components.Schemas.CabalAccessRequest.samples
    ) -> CabalAccessModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return CabalAccessModel(
            cabalID: cabal.id,
            api: APIClient(
                serverURL: serverURL, tokens: CabalAccessPreviewTokens(),
                transport: CabalAccessPreviewTransport(cabal: cabal, requests: requests)
            ),
            hints: CabalAccessPreviewHints()
        )
    }
}

private actor CabalAccessPreviewTransport: ClientTransport {
    private var cabal: Components.Schemas.Cabal
    private var requests: [Components.Schemas.CabalAccessRequest]

    init(cabal: Components.Schemas.Cabal, requests: [Components.Schemas.CabalAccessRequest]) {
        self.cabal = cabal
        self.requests = requests
    }

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let access = #"{"id":"preview","direction":"request","status":"pending"}"#
        switch operationID {
        case "getCabalAccessRequests":
            return (response, HTTPBody(try encoder.encode(requests)))
        case "postCabalAccessDecision":
            let requestID = request.path?.split(separator: "/").dropLast().last.map(String.init) ?? ""
            requests.removeAll { $0.id == requestID }
            return (response, HTTPBody(access))
        case "postCabalAccessRequest":
            cabal.myAccessRequest = .init(id: "preview", direction: "request", status: "pending")
            response.status = .created
            return (response, HTTPBody(access))
        case "deleteCabalAccessRequest":
            cabal.myAccessRequest = nil
            return (response, HTTPBody(access))
        case "postCabalMember":
            cabal.me = .init(role: "member", canVote: true)
            return (response, HTTPBody(try encoder.encode(cabal)))
        default:
            return (response, HTTPBody(try encoder.encode(cabal)))
        }
    }
}

private struct CabalAccessPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct CabalAccessPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
