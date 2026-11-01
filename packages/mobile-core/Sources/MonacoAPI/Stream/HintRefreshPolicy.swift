public struct HintRefreshPolicy: Equatable, Sendable {
    public enum Input: Equatable, Sendable {
        case hint
        case resync
        case refreshStarted
        case refreshFinished
        case becameVisible
        case becameHidden
    }

    public enum Output: Equatable, Sendable {
        case refreshNow
    }

    private var visible = true
    private var refreshInFlight = false
    private var trailingRefresh = false

    public init() {}

    public mutating func send(_ input: Input) -> Output? {
        switch input {
        case .hint, .resync:
            return noteDemand()
        case .refreshStarted:
            refreshInFlight = true
            return nil
        case .refreshFinished:
            refreshInFlight = false
            return startIfIdle()
        case .becameVisible:
            visible = true
            return startIfIdle()
        case .becameHidden:
            visible = false
            return nil
        }
    }

    private mutating func noteDemand() -> Output? {
        trailingRefresh = true
        return startIfIdle()
    }

    private mutating func startIfIdle() -> Output? {
        guard visible, !refreshInFlight, trailingRefresh else { return nil }
        trailingRefresh = false
        refreshInFlight = true
        return .refreshNow
    }
}

public enum BackgroundRefresh {
    @TaskLocal public static var isActive = false
}

@MainActor
public final class HintRefresher {
    public static let feedInterval: Duration = .seconds(5)

    private let refresh: @MainActor () async -> Void
    private let didObserveHint: @MainActor () -> Void
    private let minInterval: Duration
    private let now: () -> ContinuousClock.Instant
    private let pause: (Duration) async -> Void
    private var policy = HintRefreshPolicy()
    private var running = false
    private var lastStart: ContinuousClock.Instant?

    public init(minInterval: Duration = .zero, refresh: @escaping @MainActor () async -> Void) {
        self.refresh = refresh
        self.minInterval = minInterval
        didObserveHint = {}
        now = { ContinuousClock.now }
        pause = { try? await Task.sleep(for: $0) }
    }

    init(
        minInterval: Duration = .zero,
        now: @escaping () -> ContinuousClock.Instant = { ContinuousClock.now },
        pause: @escaping (Duration) async -> Void = { try? await Task.sleep(for: $0) },
        refresh: @escaping @MainActor () async -> Void,
        didObserveHint: @escaping @MainActor () -> Void
    ) {
        self.refresh = refresh
        self.minInterval = minInterval
        self.now = now
        self.pause = pause
        self.didObserveHint = didObserveHint
    }

    public func setVisible(_ visible: Bool) {
        let input: HintRefreshPolicy.Input = visible ? .becameVisible : .becameHidden
        guard policy.send(input) == .refreshNow else { return }
        beginRefresh()
    }

    public func observe(_ hints: AsyncStream<Hint>) async {
        await observe(hints, resyncs: true)
    }

    public func observe(_ streams: [AsyncStream<Hint>]) async {
        await withTaskGroup(of: Void.self) { group in
            for (index, stream) in streams.enumerated() {
                group.addTask { await self.observe(stream, resyncs: index == 0) }
            }
        }
    }

    private func observe(_ hints: AsyncStream<Hint>, resyncs: Bool) async {
        for await hint in hints {
            didObserveHint()
            let input: HintRefreshPolicy.Input
            if case .resync = hint {
                guard resyncs else { continue }
                input = .resync
            } else {
                input = .hint
            }
            guard policy.send(input) == .refreshNow else { continue }
            beginRefresh()
        }
    }

    private func beginRefresh() {
        guard !running else { return }
        running = true
        Task { await drain() }
    }

    private func drain() async {
        defer { running = false }
        while true {
            await waitForInterval()
            lastStart = now()
            await BackgroundRefresh.$isActive.withValue(true) { await refresh() }
            guard policy.send(.refreshFinished) == .refreshNow else { return }
        }
    }

    private func waitForInterval() async {
        guard minInterval > .zero, let last = lastStart else { return }
        let remaining = minInterval - (now() - last)
        if remaining > .zero { await pause(remaining) }
    }
}
