import Foundation
import MonacoAPI
import Observation

public struct SentCabalInvite: Identifiable, Equatable, Sendable {
    public let id: String
    public let invitee: String
    public let invitedBy: String
    public let expiry: String
    public let canRevoke: Bool
}

public struct CabalInviteToast: Equatable, Sendable {
    public let serial: Int
    public let message: String
    public let isSuccess: Bool
}

@Observable
@MainActor
public final class InviteMemberModel {
    public var handle = ""
    public private(set) var state: LoadState<[SentCabalInvite]> = .idle
    public private(set) var isSending = false
    public private(set) var revoking: Set<String> = []
    public private(set) var toast: CabalInviteToast?

    public let cabalID: String
    private let standing: CabalInviteStanding
    private let viewerID: String?
    private let api: APIClient
    private let hints: any HintSource
    private let now: @Sendable () -> Date
    private let refresher: HintRefresher
    private let sendSubmission = IdempotentSubmission()
    private let revokeSubmission = IdempotentSubmission()
    private var generation = 0
    private var toastSerial = 0

    public init(
        cabalID: String,
        standing: CabalInviteStanding,
        viewerID: String?,
        api: APIClient,
        hints: any HintSource,
        now: @escaping @Sendable () -> Date
    ) {
        self.cabalID = cabalID
        self.standing = standing
        self.viewerID = viewerID
        self.api = api
        self.hints = hints
        self.now = now
        let hook = InviteReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.load()
        }
    }

    public var canSend: Bool {
        !isSending && !CabalInvite.handle(handle).isEmpty
    }

    public func load() async {
        generation += 1
        let mine = generation
        if case .loaded = state {
        } else {
            state = .loading
        }
        let cabalID = cabalID
        do {
            let invites = try await api.read { client in
                try await client.getCabalInvites(path: .init(id: cabalID), query: .init(status: .pending)).ok.body.json
            }
            guard mine == generation else { return }
            state = .loaded(invites.map(row))
        } catch {
            guard mine == generation else { return }
            let failure = APIError(error)
            if case .loaded = state {
                show(ToastCopy.message(for: failure), success: false)
            } else {
                state = .failed(failure)
            }
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .cabal(id: cabalID, what: "members")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func send() async {
        let handle = CabalInvite.handle(handle)
        guard !isSending, !handle.isEmpty else { return }
        isSending = true
        defer { isSending = false }
        let cabalID = cabalID
        let body = Components.Schemas.InviteMemberRequest(handle: handle)
        do {
            _ = try await api.submit(
                sendSubmission, payload: SendPayload(cabalID: cabalID, handle: handle), operation: "postCabalInvite"
            ) {
                client, key in
                try await client.postCabalInvite(
                    path: .init(id: cabalID),
                    headers: .init(idempotencyKey: key),
                    body: .json(body)
                ).created.body.json
            }
            self.handle = ""
            show("Invite sent.", success: true)
            await load()
        } catch {
            show(Self.sendFailure(APIError(error)), success: false)
        }
    }

    public func revoke(_ invite: SentCabalInvite) async {
        guard invite.canRevoke, !revoking.contains(invite.id) else { return }
        revoking.insert(invite.id)
        defer { revoking.remove(invite.id) }
        let cabalID = cabalID
        let requestID = invite.id
        do {
            _ = try await api.submit(
                revokeSubmission, payload: RevokePayload(cabalID: cabalID, requestID: requestID),
                operation: "deleteCabalAccessRequest"
            ) { client, key in
                try await client.deleteCabalAccessRequest(
                    path: .init(id: cabalID, requestId: requestID),
                    headers: .init(idempotencyKey: key)
                ).ok.body.json
            }
            show("Invite revoked.", success: true)
        } catch {
            show(ToastCopy.message(for: APIError(error)), success: false)
        }
        await load()
    }

    public static func sendFailure(_ error: APIError) -> String {
        guard case .problem(let problem) = error, let outcome = Flow03Outcome(code: problem.code.wire) else {
            return ToastCopy.message(for: error)
        }
        switch outcome {
        case .userNotFound:
            return "No one on Monaco has that handle."
        case .alreadyMember:
            return "They're already in this cabal."
        case .requestPending:
            return "They already have a pending invite or request."
        case .ok, .invalidInput, .unauthorized, .cabalNotFound, .cabalBanned, .joinNeedsRequest, .requestNotNeeded,
            .notCabalCreator, .accessRequestNotPending, .cannotRevokeAccess, .inviteExpired, .notCabalMember:
            return ToastCopy.message(for: error)
        }
    }

    private func row(_ invite: Components.Schemas.CabalSentInvite) -> SentCabalInvite {
        let inviter =
            invite.invitedBy.userId == viewerID
            ? "you" : Self.name(of: invite.invitedBy)
        return SentCabalInvite(
            id: invite.requestId,
            invitee: invite.user.handle.map { "@\($0)" } ?? Self.name(of: invite.user),
            invitedBy: "Invited by \(inviter)",
            expiry: CabalInvite.expiryText(expiresAt: invite.expiresAt, now: now()),
            canRevoke: standing.canRevoke(invitedBy: invite.invitedBy.userId, viewerID: viewerID)
        )
    }

    private static func name(of person: Components.Schemas.CabalPerson) -> String {
        if !person.displayName.isEmpty { return person.displayName }
        return person.handle.map { "@\($0)" } ?? "A member"
    }

    private func show(_ message: String, success: Bool) {
        toastSerial += 1
        toast = CabalInviteToast(serial: toastSerial, message: message, isSuccess: success)
    }
}

private struct SendPayload: Encodable, Sendable {
    let cabalID: String
    let handle: String
}

private struct RevokePayload: Encodable, Sendable {
    let cabalID: String
    let requestID: String
}

private final class InviteReloadHook {
    var run: (@MainActor () async -> Void)?
}
