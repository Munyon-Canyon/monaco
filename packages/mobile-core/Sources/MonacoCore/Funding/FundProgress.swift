import Foundation
import MonacoAPI
import Observation

public enum FundProgress: Equatable, Sendable {
    case idle
    case submitting
    case submitted(FundTransfer)
    case settled(FundTransfer)
    case failed(code: String?)

    public enum Event: Equatable, Sendable {
        case start
        case accepted(FundTransfer)
        case refused
        case read(FundTransfer)
    }

    public static let funding = "Funding this cabal…"
    public static let didNotGoThrough = "Funding didn't go through. Your balance wasn't charged."

    public func applying(_ event: Event) -> FundProgress {
        switch (self, event) {
        case (.idle, .start):
            .submitting
        case (.submitting, .accepted(let transfer)):
            Self.outcome(transfer) ?? .submitted(transfer)
        case (.submitting, .refused):
            .idle
        case (.submitted(let sent), .read(let read)) where sent.transferID == read.transferID:
            Self.outcome(read) ?? .submitted(sent)
        default:
            self
        }
    }

    public var isSettled: Bool {
        switch self {
        case .settled, .failed: true
        case .idle, .submitting, .submitted: false
        }
    }

    public func toast(cabalName: String?) -> String? {
        switch self {
        case .idle, .submitting: nil
        case .submitted: Self.funding
        case .settled(let transfer):
            "Added \(Self.amount(transfer.amountMicros)) to \(cabalName ?? "the cabal")."
        case .failed: Self.didNotGoThrough
        }
    }

    private static func amount(_ micros: Int64) -> String {
        let formatted = UsdAmountFormatter.format(micros: micros)
        return micros % 1_000_000 == 0 ? String(formatted.dropLast(3)) : formatted
    }

    private static func outcome(_ transfer: FundTransfer) -> FundProgress? {
        switch transfer.status {
        case .settled: .settled(transfer)
        case .failed: .failed(code: transfer.failCode)
        case .created, .submitted, .landed: nil
        }
    }
}

public enum FundAttempt: Equatable, Sendable {
    case accepted(FundTransfer)
    case refused(FundFailure)
    case unconfirmed(retryMessage: String)
}

@Observable
@MainActor
public final class Funding {
    public private(set) var progress: FundProgress = .idle

    private let cabalID: String
    private let source: FundSource
    private let hints: any HintSource
    private let inFlightRetryDelay: Duration
    private let submission = IdempotentSubmission()

    static let inFlightRetries = 3
    public static let activityHint = "activity_changed"

    public init(
        cabalID: String, source: FundSource, hints: any HintSource, inFlightRetryDelay: Duration = .seconds(1)
    ) {
        self.cabalID = cabalID
        self.source = source
        self.hints = hints
        self.inFlightRetryDelay = inFlightRetryDelay
    }

    public var isSubmitting: Bool { progress == .submitting }

    public func submit(micros: Int64) async -> FundAttempt {
        guard progress == .idle else {
            if case .submitted(let transfer) = progress { return .accepted(transfer) }
            return .unconfirmed(retryMessage: ToastCopy.message(for: .inFlight))
        }
        progress = progress.applying(.start)
        var retries = 0
        while true {
            do {
                let transfer = try await source.fund(cabalID: cabalID, micros: micros, submission: submission)
                progress = progress.applying(.accepted(transfer))
                return .accepted(transfer)
            } catch {
                let error = APIError(error)
                if error == .inFlight, retries < Self.inFlightRetries {
                    retries += 1
                    try? await Task.sleep(for: inFlightRetryDelay)
                    continue
                }
                progress = progress.applying(.refused)
                if submission.hasPendingKey { return .unconfirmed(retryMessage: ToastCopy.message(for: error)) }
                return .refused(MoneyFlowCopy.fundCabalFailure(error))
            }
        }
    }

    public func settle() async -> FundProgress {
        guard case .submitted(let sent) = progress else { return progress }
        let (merged, signal) = AsyncStream.makeStream(of: Void.self)
        let pumps = [
            hints.hints(matching: .cabal(id: cabalID, what: Self.activityHint)),
            hints.hints(matching: .user(what: BalanceSource.refreshingHint)),
        ].map { stream in
            Task {
                for await _ in stream { signal.yield() }
            }
        }
        defer { for pump in pumps { pump.cancel() } }
        await read(sent.transferID)
        guard !progress.isSettled else { return progress }
        for await _ in merged {
            await read(sent.transferID)
            if progress.isSettled { break }
        }
        return progress
    }

    private func read(_ id: String) async {
        guard let transfer = try? await source.transfer(id: id) else { return }
        progress = progress.applying(.read(transfer))
    }
}
