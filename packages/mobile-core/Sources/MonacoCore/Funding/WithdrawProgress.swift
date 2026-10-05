import Foundation
import MonacoAPI
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
    private let submission = IdempotentSubmission()

    static let inFlightRetries = 3

    public init(source: WithdrawSource, hints: any HintSource, inFlightRetryDelay: Duration = .seconds(1)) {
        self.source = source
        self.hints = hints
        self.inFlightRetryDelay = inFlightRetryDelay
    }

    public var isSubmitting: Bool { progress == .submitting }

    public func submit(micros: Int64, toAddress: String) async -> WithdrawAttempt {
        guard progress == .idle else {
            if case .submitted(let withdrawal) = progress { return .accepted(withdrawal) }
            return .unconfirmed(retryMessage: ToastCopy.message(for: .inFlight))
        }
        progress = progress.applying(.start)
        var retries = 0
        while true {
            do {
                let withdrawal = try await source.withdraw(
                    micros: micros, toAddress: toAddress, submission: submission)
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
                if submission.hasPendingKey { return .unconfirmed(retryMessage: ToastCopy.message(for: error)) }
                return .refused(MoneyFlowCopy.withdrawFailure(error))
            }
        }
    }

    public func settle() async -> WithdrawProgress {
        guard case .submitted(let sent) = progress else { return progress }
        let stream = hints.hints(matching: .user(what: BalanceSource.refreshingHint))
        await read(sent.withdrawalID)
        guard !progress.isSettled else { return progress }
        for await _ in stream {
            await read(sent.withdrawalID)
            if progress.isSettled { break }
        }
        return progress
    }

    private func read(_ id: String) async {
        guard let withdrawal = try? await source.withdrawal(id: id) else { return }
        progress = progress.applying(.read(withdrawal))
    }
}
