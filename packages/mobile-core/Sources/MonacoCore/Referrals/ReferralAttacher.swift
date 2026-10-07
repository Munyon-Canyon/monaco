import Foundation
import MonacoAPI

public struct ReferralAttachResult: Equatable, Sendable {
    public let outcome: ReferralAttachOutcome
    public let toast: String

    public init(outcome: ReferralAttachOutcome, toast: String) {
        self.outcome = outcome
        self.toast = toast
    }
}

private struct AttachPayload: Encodable, Sendable {
    let code: String
    let source: ReferralSource
}

@MainActor
public final class ReferralAttacher {
    public static let attachedUserKey = "monaco.referralAttachedUser"

    private let api: APIClient
    private let pending: PendingReferralStore
    private let store: any KeyValueStoring
    private let now: @Sendable () -> Date
    private var submissions: [String: IdempotentSubmission] = [:]
    private var isAttaching = false

    public init(api: APIClient, store: any KeyValueStoring, now: @escaping @Sendable () -> Date) {
        self.api = api
        self.store = store
        self.now = now
        pending = PendingReferralStore(store: store)
    }

    public var hasPending: Bool { pending.load(now: now()) != nil }

    public func hasAttached(userID: String) -> Bool {
        store.data(forKey: Self.attachedUserKey) == Data(userID.utf8)
    }

    public func offersManualEntry(userID: String) -> Bool {
        !hasPending && !hasAttached(userID: userID)
    }

    public func attachPending(userID: String) async -> ReferralAttachResult? {
        guard !isAttaching, let referral = pending.load(now: now()) else { return nil }
        isAttaching = true
        defer { isAttaching = false }
        let result = await send(referral.code, source: referral.source, userID: userID)
        guard result.outcome != .retryLater else { return nil }
        pending.clear()
        return result
    }

    public func attach(_ code: ReferralCode, userID: String) async -> ReferralAttachResult {
        await send(code, source: .manual, userID: userID)
    }

    private func send(_ code: ReferralCode, source: ReferralSource, userID: String) async -> ReferralAttachResult {
        let slot = "\(code.value)/\(source.rawValue)"
        let submission = submissions[slot] ?? IdempotentSubmission()
        submissions[slot] = submission
        let body = Components.Schemas.AttachReferralRequest(code: code.value, source: Self.wire(source))
        do {
            let attached = try await api.submit(
                submission, payload: AttachPayload(code: code.value, source: source), operation: "postMeReferral"
            ) { client, key in
                try await client.postMeReferral(headers: .init(idempotencyKey: key), body: .json(body))
                    .created.body.json
            }
            markAttached(userID)
            return ReferralAttachResult(
                outcome: ReferralAttachOutcome.from(problem: nil),
                toast: ReferralCopy.joined(handle: attached.referrer.handle))
        } catch {
            let error = APIError(error)
            if case .problem(let problem) = error, problem.code == .known(.referralAlreadyAttached) {
                markAttached(userID)
            }
            return ReferralAttachResult(
                outcome: ReferralAttachOutcome.from(problem: error), toast: ToastCopy.message(for: error))
        }
    }

    private func markAttached(_ userID: String) {
        store.set(Data(userID.utf8), forKey: Self.attachedUserKey)
    }

    private static func wire(_ source: ReferralSource) -> Components.Schemas.ReferralSource {
        switch source {
        case .universalLink: .universalLink
        case .clipboard: .clipboard
        case .manual: .manual
        }
    }
}
