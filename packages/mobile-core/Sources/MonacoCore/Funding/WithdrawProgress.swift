import Foundation
import MonacoAPI
import MonacoFunding
import Observation

public enum WithdrawProgress: Equatable, Sendable {
    case idle
    case submitting
    case submitted(Withdrawal)
    case confirmed(Withdrawal)
    case failed(code: String?)

    public enum Event: Equatable, Sendable {
        case start
        case accepted(Withdrawal)
        case refused
        case read(Withdrawal)
    }

    public static let didNotGoThrough = "Withdrawal didn't go through. Your balance wasn't charged."

    public func applying(_ event: Event) -> WithdrawProgress {
        switch (self, event) {
        case (.idle, .start):
            .submitting
        case (.submitting, .accepted(let withdrawal)):
            Self.settled(withdrawal) ?? .submitted(withdrawal)
        case (.submitting, .refused):
            .idle
        case (.submitted(let sent), .read(let read)) where sent.withdrawalID == read.withdrawalID:
            Self.settled(read) ?? .submitted(sent)
        default:
            self
        }
    }

    public var isSettled: Bool {
        switch self {
        case .confirmed, .failed: true
        case .idle, .submitting, .submitted: false
        }
    }

    public var toast: String? {
        switch self {
        case .idle, .submitting: nil
        case .submitted(let withdrawal):
            "Withdrawing \(UsdAmountFormatter.format(micros: withdrawal.amountMicros)). It lands in about a minute."
        case .confirmed(let withdrawal):
            "Withdrawal complete: \(UsdAmountFormatter.format(micros: withdrawal.amountMicros))"
        case .failed:
            Self.didNotGoThrough
        }
    }

    private static func settled(_ withdrawal: Withdrawal) -> WithdrawProgress? {
        switch withdrawal.status {
        case .confirmed: .confirmed(withdrawal)
        case .failed: .failed(code: withdrawal.failCode)
        case .created, .submitted: nil
        }
    }
}

public enum WithdrawAttempt: Equatable, Sendable {
    case accepted(Withdrawal)
    case refused(WithdrawFailure)
    case unconfirmed(retryMessage: String)
}

@Observable
@MainActor
public final class Withdrawing {
    public private(set) var progress: WithdrawProgress = .idle

    private let source: WithdrawSource
    private let hints: any HintSource
    private let inFlightRetryDelay: Duration
    private let submission: IdempotentSubmission
    private let schedule: ReconcileSchedule

    public struct Pending: Codable, Equatable, Sendable {
        public let micros: Int64
        public let toAddress: String
        public let network: String
    }

    static let inFlightRetries = 3

    public init(
        source: WithdrawSource, hints: any HintSource, inFlightRetryDelay: Duration = .seconds(1),
        submission: IdempotentSubmission = IdempotentSubmission(), schedule: ReconcileSchedule = .standard()
    ) {
        self.source = source
        self.hints = hints
        self.inFlightRetryDelay = inFlightRetryDelay
        self.submission = submission
        self.schedule = schedule
    }

    public static let slot = "withdraw"

    public var isSubmitting: Bool { progress == .submitting }

    public var pending: Pending? { submission.hasPendingKey ? submission.pending(Pending.self) : nil }

    public var pendingMicros: Int64? { pending?.micros }

    public func submit(
        micros: Int64, toAddress: String, network: WithdrawNetwork = .solana
    ) async -> WithdrawAttempt {
        guard progress == .idle else {
            if case .submitted(let withdrawal) = progress { return .accepted(withdrawal) }
            return .unconfirmed(retryMessage: ToastCopy.message(for: .inFlight))
        }
        let request = Pending(micros: micros, toAddress: toAddress, network: network.rawValue)
        if submission.hasPendingKey, let pending = submission.pending(Pending.self), pending != request {
            return .unconfirmed(retryMessage: MoneyFlowCopy.unconfirmed.summary)
        }
        submission.stage(request)
        progress = progress.applying(.start)
        var retries = 0
        while true {
            do {
                let withdrawal = try await source.withdraw(
                    micros: micros, toAddress: toAddress, network: network, submission: submission)
                progress = progress.applying(.accepted(withdrawal))
                return .accepted(withdrawal)
            } catch {
                let error = APIError(error)
                if error == .inFlight, retries < Self.inFlightRetries {
                    retries += 1
                    try? await Task.sleep(for: inFlightRetryDelay)
                    continue
                }
                progress = progress.applying(.refused)
                if submission.hasPendingKey {
                    return .unconfirmed(retryMessage: MoneyFlowCopy.pendingKeyLine(error))
                }
                return .refused(MoneyFlowCopy.withdrawFailure(error))
            }
        }
    }

    func resume(withdrawalID: String) {
        guard progress == .idle else { return }
        progress = .submitted(
            Withdrawal(withdrawalID: withdrawalID, status: .submitted, amountMicros: 0, txSignature: nil))
    }

    public func settle() async -> WithdrawProgress {
        guard case .submitted(let sent) = progress else { return progress }
        await SettlementReconciler.run(
            hints: [hints.hints(matching: .user(what: BalanceSource.refreshingHint))], schedule: schedule,
            read: { await read(sent.withdrawalID) }, isDone: { progress.isSettled })
        return progress
    }

    private func read(_ id: String) async -> Bool {
        guard let withdrawal = try? await source.withdrawal(id: id) else { return false }
        progress = progress.applying(.read(withdrawal))
        return true
    }
}
