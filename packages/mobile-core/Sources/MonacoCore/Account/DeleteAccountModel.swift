import MonacoAPI
import Observation

public struct DeleteAccountChecklist: Equatable, Sendable {
    public let balance: AccountBalance
    public let cabals: [Components.Schemas.MyCabal]

    public init(balance: AccountBalance, cabals: [Components.Schemas.MyCabal]) {
        self.balance = balance
        self.cabals = cabals
    }

    public var isCashedOut: Bool { cabals.isEmpty }
    public var isWithdrawn: Bool { balance.availableMicros == 0 && balance.inFlightMicros == 0 }

    public func isDone(_ step: DeleteAccountBlocker) -> Bool {
        switch step {
        case .cashOutFirst: isCashedOut
        case .withdrawFirst: isWithdrawn
        }
    }
}

@Observable
@MainActor
public final class DeleteAccountModel {
    public private(set) var state: LoadState<DeleteAccountChecklist> = .idle
    public private(set) var isDeleting = false
    public private(set) var isDeleted = false
    public private(set) var blocker: DeleteAccountBlocker?
    public private(set) var toastMessage: String?
    public private(set) var failureTick = 0

    private let api: APIClient
    private let sessions: SessionAPI
    private let submission = IdempotentSubmission()
    private var generation = 0

    public init(api: APIClient) {
        self.api = api
        sessions = SessionAPI(api: api)
    }

    public var checklist: DeleteAccountChecklist? {
        if case .loaded(let checklist) = state { return checklist }
        return nil
    }

    public var highlighted: DeleteAccountBlocker? {
        guard let blocker, checklist?.isDone(blocker) != true else { return nil }
        return blocker
    }

    public func load() async {
        generation += 1
        let issued = generation
        if checklist == nil { state = .loading }
        do {
            async let balance = api.read { try await $0.getMyBalance().ok.body.json }
            async let cabals = api.read { try await $0.getMyCabals().ok.body.json }
            let loaded = DeleteAccountChecklist(
                balance: try AccountBalance(try await balance), cabals: try await cabals)
            guard issued == generation else { return }
            state = .loaded(loaded)
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            let error = APIError(error)
            if checklist == nil {
                state = .failed(error)
            } else {
                fail(ToastCopy.message(for: error))
            }
        }
    }

    public func delete() async {
        guard !isDeleting, !isDeleted else { return }
        isDeleting = true
        defer { isDeleting = false }
        do {
            try await sessions.deleteAccount(submission: submission)
            blocker = nil
            isDeleted = true
        } catch {
            let error = APIError(error)
            if case .accountDeleted = error {
                isDeleted = true
                return
            }
            blocker = DeleteAccountState.state(for: error)
            fail(blocker?.message ?? ToastCopy.message(for: error))
        }
    }

    private func fail(_ message: String) {
        toastMessage = message
        failureTick += 1
    }
}
