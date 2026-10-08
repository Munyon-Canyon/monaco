import MonacoAPI

public actor PnLHistoryLoader {
    public enum Subject: Hashable, Sendable {
        case me
        case cabal(id: String)
    }

    public struct Key: Hashable, Sendable {
        public let subject: Subject
        public let range: LeaderboardRange
    }

    public enum Update: Sendable {
        case refreshed(range: LeaderboardRange, curves: [Subject: ValueCurve])
        case failed(APIError)
    }

    static let maxInFlight = 4
    static let maxRetryAfter = 5

    private let api: APIClient
    private let hints: any HintSource
    private let sleep: @Sendable (Duration) async throws -> Void
    private var cache: [Key: ValueCurve] = [:]
    private var visible: [Key] = []
    private var generation = 0
    private var tail: Task<Void, Never>?
    private var policy = HintRefreshPolicy()
    private var onUpdate: (@Sendable (Update) async -> Void)?

    public init(
        api: APIClient,
        hints: any HintSource,
        sleep: @escaping @Sendable (Duration) async throws -> Void = { try await Task.sleep(for: $0) }
    ) {
        self.api = api
        self.hints = hints
        self.sleep = sleep
    }

    public var visibleRange: LeaderboardRange? { visible.first?.range }

    public func cached(_ subject: Subject, range: LeaderboardRange) -> ValueCurve? {
        cache[Key(subject: subject, range: range)]
    }

    public func show(_ subject: Subject, range: LeaderboardRange) async throws -> ValueCurve? {
        try await show([subject], range: range)?[subject]
    }

    public func show(_ subjects: [Subject], range: LeaderboardRange) async throws -> [Subject: ValueCurve]? {
        let keys = subjects.map { Key(subject: $0, range: range) }
        visible = keys
        generation += 1
        if keys.allSatisfy({ cache[$0] != nil }) {
            var curves: [Subject: ValueCurve] = [:]
            for key in keys { curves[key.subject] = cache[key] }
            return curves
        }
        return try await queued(keys, issued: generation, force: false)
    }

    public func setVisible(_ isVisible: Bool) async {
        await note(isVisible ? .becameVisible : .becameHidden)
    }

    public func observe(onUpdate: @escaping @Sendable (Update) async -> Void) async {
        self.onUpdate = onUpdate
        let user = hints.hints(matching: .user(what: nil))
        let board = hints.hints(matching: .global(what: "leaderboards_updated"))
        await withTaskGroup(of: Void.self) { group in
            group.addTask { await self.consume(user, resyncs: true) }
            group.addTask { await self.consume(board, resyncs: false) }
        }
    }

    private func consume(_ stream: AsyncStream<Hint>, resyncs: Bool) async {
        for await hint in stream {
            if case .resync = hint {
                if resyncs { await note(.resync) }
            } else {
                await note(.hint)
            }
        }
    }

    private func note(_ input: HintRefreshPolicy.Input) async {
        guard policy.send(input) == .refreshNow else { return }
        repeat {
            await refetchVisible()
        } while policy.send(.refreshFinished) == .refreshNow
    }

    private func refetchVisible() async {
        guard !visible.isEmpty else { return }
        let keys = visible
        cache.removeAll()
        do {
            guard let curves = try await queued(keys, issued: generation, force: true) else { return }
            await onUpdate?(.refreshed(range: keys[0].range, curves: curves))
        } catch {
            await onUpdate?(.failed(APIError(error)))
        }
    }

    private func queued(_ keys: [Key], issued: Int, force: Bool) async throws -> [Subject: ValueCurve]? {
        let previous = tail
        let task = Task { () async throws -> [Subject: ValueCurve]? in
            await previous?.value
            return try await self.fetch(keys, issued: issued, force: force)
        }
        tail = Task { _ = try? await task.value }
        return try await task.value
    }

    private func fetch(_ keys: [Key], issued: Int, force: Bool) async throws -> [Subject: ValueCurve]? {
        guard issued == generation else { return nil }
        let missing = force ? keys : keys.filter { cache[$0] == nil }
        if !missing.isEmpty {
            let api = api
            let sleep = sleep
            let results = await withTaskGroup(of: (Key, Result<ValueCurve, any Error>).self) { group in
                var pending = missing[...]
                func addNext() {
                    guard let key = pending.popFirst() else { return }
                    group.addTask {
                        do {
                            return (key, .success(try await Self.read(key, api: api, sleep: sleep)))
                        } catch {
                            return (key, .failure(error))
                        }
                    }
                }
                for _ in 0..<Self.maxInFlight { addNext() }
                var all: [(Key, Result<ValueCurve, any Error>)] = []
                while let result = await group.next() {
                    all.append(result)
                    addNext()
                }
                return all
            }
            var firstFailure: (any Error)?
            var fetched = 0
            for (key, result) in results {
                switch result {
                case .success(let curve):
                    cache[key] = curve
                    fetched += 1
                case .failure(let error):
                    firstFailure = firstFailure ?? error
                }
            }
            if fetched == 0, let firstFailure { throw firstFailure }
        }
        guard issued == generation else { return nil }
        var curves: [Subject: ValueCurve] = [:]
        for key in keys { curves[key.subject] = cache[key] }
        return curves
    }

    private nonisolated static func read(
        _ key: Key,
        api: APIClient,
        sleep: @Sendable (Duration) async throws -> Void
    ) async throws -> ValueCurve {
        do {
            return try await read(key, api: api)
        } catch APIError.problem(let problem) where problem.status == 429 {
            let seconds = min(max(problem.retryAfterSeconds ?? 1, 1), maxRetryAfter)
            try await sleep(.seconds(seconds))
            return try await read(key, api: api)
        }
    }

    private nonisolated static func read(_ key: Key, api: APIClient) async throws -> ValueCurve {
        switch key.subject {
        case .me:
            let history = try await api.read {
                try await $0.getMyPnlHistory(query: .init(range: key.range.valueHistory)).ok.body.json
            }
            return ValueCurve(history)
        case .cabal(let id):
            let history = try await api.read {
                try await $0.getCabalValueHistory(path: .init(id: id), query: .init(range: key.range.valueHistory))
                    .ok.body.json
            }
            return ValueCurve(history)
        }
    }
}
