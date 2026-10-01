import Foundation

/// Shared poll cadence for inbound USDC balance and fund-to-cabal sweep status.
public enum DepositPolling {
    public static let balanceInterval: Duration = .seconds(3)
    public static let sweepStatusInterval: Duration = .seconds(3)
    /// Stop polling fund sweeps after this long; backend poller keeps working.
    public static let sweepMaxWait: Duration = .seconds(120)
}

/// Normalizes backend deposit status strings (`failed`, `failed: submit_sweep`, …).
public enum DepositStatusNormalizer {
    public static func isPending(_ status: String) -> Bool {
        status.lowercased() == "pending"
    }

    public static func isConfirmed(_ status: String) -> Bool {
        status.lowercased() == "confirmed"
    }

    public static func isFailed(_ status: String) -> Bool {
        let normalized = status.lowercased()
        return normalized == "failed" || normalized.hasPrefix("failed:")
    }
}

public enum DepositSweepPhase: Equatable {
    case idle
    case awaitingSweep
    case credited
    case failed(String)
}

/// Tracks deposit sweep status from backend DTO `status` strings.
public struct DepositPollStateMachine: Equatable {
    public private(set) var phase: DepositSweepPhase = .idle

    public init() {}

    public mutating func apply(status: String) {
        if DepositStatusNormalizer.isPending(status) {
            phase = .awaitingSweep
        } else if DepositStatusNormalizer.isConfirmed(status) {
            phase = .credited
        } else if DepositStatusNormalizer.isFailed(status) {
            phase = .failed(status)
        } else {
            phase = .failed(status)
        }
    }

    /// Polls `fetchStatus` until the deposit reaches a terminal phase or `maxWait` elapses.
    /// `clock` defaults to the live clock. Tests pass a clock they advance.
    public mutating func pollUntilTerminal(
        maxWait: Duration = DepositPolling.sweepMaxWait,
        interval: Duration = DepositPolling.sweepStatusInterval,
        clock: any Clock<Duration> = ContinuousClock(),
        fetchStatus: () async throws -> String
    ) async -> DepositSweepPhase {
        let timer = PollClock(clock)
        while timer.now() < maxWait {
            if Task.isCancelled {
                return phase
            }
            do {
                apply(status: try await fetchStatus())
                if isTerminal {
                    return phase
                }
            } catch {
                // Keep last known phase; retry on transient auth/network/decode errors.
            }
            try? await timer.sleep(interval)
        }
        return phase
    }

    public var isTerminal: Bool {
        switch phase {
        case .credited, .failed:
            true
        case .idle, .awaitingSweep:
            false
        }
    }
}

/// Elapsed time and sleep for one `pollUntilTerminal` run, so the machine can take any clock.
private struct PollClock: Sendable {
    let now: @Sendable () -> Duration
    let sleep: @Sendable (Duration) async throws -> Void

    init(_ clock: any Clock<Duration>) {
        self = Self.opening(clock)
    }

    private init(now: @escaping @Sendable () -> Duration, sleep: @escaping @Sendable (Duration) async throws -> Void) {
        self.now = now
        self.sleep = sleep
    }

    private static func opening<C: Clock<Duration>>(_ clock: C) -> PollClock {
        let origin = clock.now
        return PollClock(now: { origin.duration(to: clock.now) }, sleep: { try await clock.sleep(for: $0) })
    }
}

public struct DepositStatusDTO: Decodable, Equatable {
    public let depositId: String
    public let status: String
    public let shareUnits: Int64

    public init(depositId: String, status: String, shareUnits: Int64) {
        self.depositId = depositId
        self.status = status
        self.shareUnits = shareUnits
    }
}
