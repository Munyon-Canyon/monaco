import Foundation
import MonacoAPI
import MonacoGovernance
import Observation

@Observable
@MainActor
public final class ProposalRemindModel {
    public private(set) var isOn: Bool
    public private(set) var isSending = false
    public private(set) var errorMessage: String?
    private let proposalID: String
    private let repository: ProposalsRepository
    private var setSubmission = IdempotentSubmission()
    private var clearSubmission = IdempotentSubmission()

    public init(proposalID: String, initial: Bool, repository: ProposalsRepository) {
        self.proposalID = proposalID
        isOn = initial
        self.repository = repository
    }

    public static let offToast = "Reminder off."

    public static func toastAfterOn(dueAt: Date?, now: Date) -> String {
        if let dueAt, dueAt.timeIntervalSince(now) < 60 {
            return "It closes within the hour. Reminder on."
        }
        return "We'll remind you an hour before it closes."
    }

    public func apply(server: Bool) {
        guard !isSending else { return }
        isOn = server
    }

    @discardableResult
    public func toggle() async -> ProposalReminderState? {
        guard !isSending else { return nil }
        let wasOn = isOn
        isOn = !wasOn
        isSending = true
        errorMessage = nil
        defer { isSending = false }
        do {
            let state =
                wasOn
                ? try await repository.clearReminder(id: proposalID, submission: clearSubmission)
                : try await repository.setReminder(id: proposalID, submission: setSubmission)
            if wasOn { clearSubmission = IdempotentSubmission() } else { setSubmission = IdempotentSubmission() }
            isOn = state.isSet
            return state
        } catch {
            isOn = wasOn
            errorMessage = ToastCopy.message(for: APIError(error))
            return nil
        }
    }
}
