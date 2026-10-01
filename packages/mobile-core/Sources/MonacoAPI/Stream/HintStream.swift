import Foundation
import HTTPTypes
import OpenAPIRuntime
import OpenAPIURLSession

/// The app's one `GET /v1/stream` connection, fanned out in memory to every subscriber. It
/// reconnects with backoff, treats 45 s without a byte as a dead connection, and emits
/// `.resync` on every connect and every 30 s while it cannot connect, so a screen re-fetches
/// whenever it may have missed a hint.
public actor HintStream: HintSource {
    public enum State: Hashable, Sendable {
        case stopped
        case connecting
        case connected
        case reconnecting
        case signedOut
    }

    static let heartbeatTimeout: Duration = .seconds(45)
    static let fallbackInterval: Duration = .seconds(30)
    static let backoffFloor: Duration = .seconds(1)
    static let backoffCeiling: Duration = .seconds(30)
    static let backoffReset: Duration = .seconds(60)

    public private(set) var state: State = .stopped
    /// Hint events whose key or `what` did not parse.
    public private(set) var droppedCount = 0

    private let serverURL: URL
    private let transport: any ClientTransport
    private let token: @Sendable () async throws -> String?
    private let refresh: @Sendable (String) async throws -> String?
    private let timer: StreamClock
    private let random: @Sendable () -> Double
    private let subscribers = Subscribers()
    private var lastEventID: String?
    private var lastByteAt: Duration = .zero
    private var run: Task<Void, Never>?
    private var fallback: Task<Void, Never>?

    public init(
        serverURL: URL,
        transport: any ClientTransport = URLSessionTransport(),
        token: @escaping @Sendable () async throws -> String?,
        refresh: @escaping @Sendable (String) async throws -> String?,
        clock: any Clock<Duration> = ContinuousClock(),
        random: @escaping @Sendable () -> Double = { Double.random(in: 0..<1) }
    ) {
        self.serverURL = serverURL
        self.transport = transport
        self.token = token
        self.refresh = refresh
        self.timer = StreamClock(clock)
        self.random = random
    }

    public nonisolated func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        subscribers.add(filter)
    }

    public func start() {
        guard run == nil || state == .signedOut else { return }
        run = Task { await self.connectUntilStopped() }
    }

    public func stop() {
        run?.cancel()
        run = nil
        stopFallback()
        state = .stopped
    }

    private func connectUntilStopped() async {
        var attempt = 0
        var refreshed: String?
        var rejected = false
        startFallback()
        while !Task.isCancelled {
            set(.connecting)
            var bearer = refreshed
            refreshed = nil
            var connectedFor = Duration.zero
            do {
                if bearer == nil { bearer = try await token() }
                connectedFor = try await connect(bearer)
                rejected = false
            } catch let error where ProblemError(error)?.status == 401 {
                guard !rejected, let bearer else { return signOut() }
                rejected = true
                do {
                    guard let fresh = try await refresh(bearer) else { return signOut() }
                    refreshed = fresh
                    continue
                } catch {
                    rejected = false
                }
            } catch {}
            if connectedFor >= Self.backoffReset { attempt = 0 }
            let delay = Self.backoff(attempt) * random()
            attempt += 1
            set(.reconnecting)
            startFallback()
            try? await timer.sleep(delay)
        }
    }

    static func backoff(_ attempt: Int) -> Duration {
        min(backoffFloor * (1 << min(attempt, 5)), backoffCeiling)
    }

    private func connect(_ bearer: String?) async throws -> Duration {
        let client = Client(
            serverURL: serverURL,
            transport: transport,
            middlewares: [HeadersMiddleware(accessToken: { bearer }), ProblemMiddleware()]
        )
        let body = try await client.getStream(headers: .init(lastEventID: lastEventID)).ok.body.textEventStream
        guard !Task.isCancelled else { return .zero }
        let connectedAt = timer.elapsed()
        lastByteAt = connectedAt
        stopFallback()
        set(.connected)
        subscribers.yield(.resync)
        try? await withThrowingTaskGroup(of: Void.self) { group in
            group.addTask { try await self.read(body) }
            group.addTask { try await self.watchHeartbeat() }
            try await group.next()
            group.cancelAll()
        }
        return timer.elapsed() - connectedAt
    }

    private func read(_ body: HTTPBody) async throws {
        let chunks = body.map { chunk in
            await self.sawBytes()
            return chunk
        }
        for try await event in chunks.asDecodedServerSentEvents(while: { _ in true }) {
            receive(event)
        }
    }

    private func sawBytes() {
        lastByteAt = timer.elapsed()
    }

    private func watchHeartbeat() async throws {
        while true {
            let idle = timer.elapsed() - lastByteAt
            if idle >= Self.heartbeatTimeout { throw HeartbeatTimeout() }
            try await timer.sleep(Self.heartbeatTimeout - idle)
        }
    }

    private func receive(_ event: ServerSentEvent) {
        guard event.event == "hint", !Task.isCancelled else { return }
        if let id = event.id { lastEventID = id }
        guard let data = event.data,
            let wire = try? JSONDecoder().decode(WireHint.self, from: Data(data.utf8)),
            let hint = Hint(key: wire.key, what: wire.what, id: event.id ?? "")
        else {
            droppedCount += 1
            return
        }
        subscribers.yield(hint)
    }

    private func set(_ state: State) {
        guard !Task.isCancelled else { return }
        self.state = state
    }

    private func signOut() {
        guard !Task.isCancelled else { return }
        stopFallback()
        state = .signedOut
    }

    private func startFallback() {
        guard fallback == nil, !Task.isCancelled else { return }
        fallback = Task { [timer, subscribers] in
            while (try? await timer.sleep(Self.fallbackInterval)) != nil {
                subscribers.yield(.resync)
            }
        }
    }

    private func stopFallback() {
        fallback?.cancel()
        fallback = nil
    }
}

private struct WireHint: Decodable {
    let key: String
    let what: String
}

private struct HeartbeatTimeout: Error {}

/// `any Clock<Duration>` opened once, so the stream can read elapsed time without knowing the
/// clock's `Instant` type.
private struct StreamClock: Sendable {
    let elapsed: @Sendable () -> Duration
    let sleep: @Sendable (Duration) async throws -> Void

    init(_ clock: any Clock<Duration>) {
        self = Self.opening(clock)
    }

    private init(
        elapsed: @escaping @Sendable () -> Duration, sleep: @escaping @Sendable (Duration) async throws -> Void
    ) {
        self.elapsed = elapsed
        self.sleep = sleep
    }

    private static func opening<C: Clock<Duration>>(_ clock: C) -> StreamClock {
        let start = clock.now
        return StreamClock(elapsed: { start.duration(to: clock.now) }, sleep: { try await clock.sleep(for: $0) })
    }
}

private final class Subscribers: @unchecked Sendable {
    private let lock = NSLock()
    private var next = 0
    private var entries: [Int: (filter: HintFilter, continuation: AsyncStream<Hint>.Continuation)] = [:]

    func add(_ filter: HintFilter) -> AsyncStream<Hint> {
        let (stream, continuation) = AsyncStream.makeStream(of: Hint.self, bufferingPolicy: .bufferingNewest(32))
        lock.lock()
        let id = next
        next += 1
        entries[id] = (filter, continuation)
        lock.unlock()
        continuation.onTermination = { [weak self] _ in self?.remove(id) }
        return stream
    }

    func yield(_ hint: Hint) {
        lock.lock()
        let targets = entries.values.filter { $0.filter.matches(hint) }.map(\.continuation)
        lock.unlock()
        for continuation in targets { continuation.yield(hint) }
    }

    private func remove(_ id: Int) {
        lock.lock()
        entries[id] = nil
        lock.unlock()
    }
}
