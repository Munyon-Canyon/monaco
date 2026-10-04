import Foundation
import MonacoAPI

public actor HandleAvailabilityChecker {
    public static let debounce = Duration.milliseconds(400)
    static let fallbackRetryAfter = Duration.seconds(1)

    public private(set) var status = HandleStatus.idle
    public nonisolated let statuses: AsyncStream<HandleStatus>

    private let check: @Sendable (String) async throws -> HandleStatus
    private let sleep: @Sendable (Duration) async throws -> Void
    private let continuation: AsyncStream<HandleStatus>.Continuation
    private var input = ""
    private var pending: Task<Void, Never>?

    public init(sessions: SessionAPI, clock: some Clock<Duration>) {
        self.check = { try await sessions.handleAvailability($0) }
        self.sleep = { try await clock.sleep(for: $0) }
        (statuses, continuation) = AsyncStream.makeStream(bufferingPolicy: .bufferingNewest(1))
    }

    deinit {
        pending?.cancel()
        continuation.finish()
    }

    public func update(_ raw: String) {
        let handle = HandleInput.normalize(raw)
        guard handle != input else { return }
        input = handle
        pending?.cancel()
        pending = nil
        if handle.isEmpty {
            publish(.idle)
        } else if let reason = HandleInput.localReason(handle) {
            publish(.unavailable(handle, reason))
        } else {
            publish(.checking(handle))
            pending = Task { await run(handle, after: Self.debounce) }
        }
    }

    public func retry() {
        guard case .failed(let handle) = status, handle == input else { return }
        pending?.cancel()
        publish(.checking(handle))
        pending = Task { await run(handle, after: .zero) }
    }

    private func run(_ handle: String, after delay: Duration) async {
        var wait = delay
        while true {
            do {
                if wait > .zero { try await sleep(wait) }
                let verdict = try await check(handle)
                guard isCurrent(handle) else { return }
                publish(verdict)
                return
            } catch {
                guard isCurrent(handle) else { return }
                guard case .problem(let problem) = APIError(error), problem.status == 429 else {
                    publish(.failed(handle))
                    return
                }
                publish(.slowDown(handle))
                wait = problem.retryAfterSeconds.map { .seconds($0) } ?? Self.fallbackRetryAfter
            }
        }
    }

    private func isCurrent(_ handle: String) -> Bool {
        !Task.isCancelled && handle == input
    }

    private func publish(_ next: HandleStatus) {
        status = next
        continuation.yield(next)
    }
}
