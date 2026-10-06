import Foundation
import MonacoAPI

public enum ChatLoad: Equatable, Sendable {
    case idle
    case loading
    case loaded
    case failed(APIError)
}

public actor ChatSession {
    public struct Notice: Equatable, Sendable {
        public let serial: Int
        public let error: APIError
    }

    public struct State: Equatable, Sendable {
        public var load: ChatLoad = .idle
        public var timeline: ChatTimeline
        public var isClosed = false
        public var isLoadingOlder = false
        public var notice: Notice?
    }

    public static let pageSize = 50
    static let maxCatchUpPages = 10

    public nonisolated let cabalID: String
    let api: APIClient
    private let realtime: any ChatRealtime
    let now: @Sendable () -> Date
    let makeKey: @Sendable () -> String
    var state: State
    private var subscribers: [UUID: AsyncStream<State>.Continuation] = [:]
    private var submissions: [String: IdempotentSubmission] = [:]
    private var listener: Task<Void, Never>?
    var threads: [String: WeakThread] = [:]
    var attached: [String: ThreadSession] = [:]
    var deletions: [String: IdempotentSubmission] = [:]
    var isOpen = false
    private var noticeSerial = 0

    public init(
        cabalID: String,
        viewerID: String,
        api: APIClient,
        realtime: any ChatRealtime,
        now: @escaping @Sendable () -> Date,
        makeKey: @escaping @Sendable () -> String = { UUID().uuidString.lowercased() }
    ) {
        self.cabalID = cabalID
        self.api = api
        self.realtime = realtime
        self.now = now
        self.makeKey = makeKey
        self.state = State(timeline: ChatTimeline(viewerID: viewerID))
    }

    public func states() -> AsyncStream<State> {
        let id = UUID()
        let (stream, continuation) = AsyncStream.makeStream(of: State.self, bufferingPolicy: .bufferingNewest(1))
        subscribers[id] = continuation
        continuation.onTermination = { [weak self] _ in
            Task { await self?.removeSubscriber(id) }
        }
        continuation.yield(state)
        return stream
    }

    public func open() async {
        guard !isOpen else { return }
        isOpen = true
        if state.timeline.newestID == nil {
            await loadNewest()
        } else {
            await catchUp()
        }
        guard isOpen, !state.isClosed else { return }
        subscribe()
    }

    public func close() {
        isOpen = false
        if attached.isEmpty { stopListening() }
    }

    func newKey() -> String { makeKey() }

    public func reload() async {
        await loadNewest()
        if isOpen, !state.isClosed { subscribe() }
    }

    public func loadOlder() async {
        guard let cursor = state.timeline.oldestID, state.timeline.hasOlder, !state.isLoadingOlder else { return }
        state.isLoadingOlder = true
        publish()
        do {
            let page = try await fetch(before: cursor)
            state.timeline.mergeOlder(page, pageSize: Self.pageSize)
        } catch {
            fail(error, firstLoad: false)
        }
        state.isLoadingOlder = false
        publish()
    }

    @discardableResult
    public func send(
        body: String, parentId: String? = nil, alsoInChannel: Bool = false
    ) async -> GroupChatDraft.Problem? {
        if let parentId {
            return await thread(parentId: parentId).send(body: body, alsoInChannel: alsoInChannel)
        }
        let trimmed: String
        switch GroupChatDraft.validate(body) {
        case .success(let text): trimmed = text
        case .failure(let problem): return problem
        }
        let key = makeKey()
        submissions[key] = IdempotentSubmission(makeKey: { key })
        state.timeline.addUnsent(key: key, body: trimmed, at: now())
        publish()
        await deliver(key: key)
        return nil
    }

    public func retry(key: String) async {
        guard state.timeline.unsent(key: key)?.failed == true else { return }
        state.timeline.setFailed(key: key, false)
        publish()
        await deliver(key: key)
    }

    func apply(_ event: ChatRealtimeEvent) async {
        for thread in attached.values { await thread.apply(event) }
        switch event {
        case .messageCreated(let message):
            guard state.timeline.hasLoadedNewest else { return }
            state.timeline.insertLive(message)
        case .threadUpdated(let id, let replyCount, let lastReplyAt):
            state.timeline.applyThread(id: id, replyCount: replyCount, lastReplyAt: lastReplyAt)
        case .messageDeleted(let id):
            state.timeline.markDeleted(id: id)
        case .attached(let resumed):
            if !resumed { await catchUp() }
        case .seenUpdated, .detached:
            return
        }
        publish()
    }

    func stopListening() {
        listener?.cancel()
        listener = nil
        realtime.detach(cabalId: cabalID)
    }

    func subscribe() {
        guard listener == nil else { return }
        let stream = realtime.events(cabalId: cabalID)
        listener = Task { [weak self] in
            for await event in stream {
                guard let self else { return }
                await self.apply(event)
            }
        }
    }

    private func loadNewest() async {
        if state.load != .loaded { state.load = .loading }
        publish()
        do {
            let page = try await fetch()
            state.timeline.mergeNewest(page, pageSize: Self.pageSize)
            state.load = .loaded
            state.isClosed = false
        } catch {
            fail(error, firstLoad: !state.timeline.hasLoadedNewest)
        }
        publish()
    }

    private func catchUp() async {
        guard var cursor = state.timeline.newestID else {
            await loadNewest()
            return
        }
        for _ in 0..<Self.maxCatchUpPages {
            do {
                let page = try await fetch(after: cursor)
                state.timeline.mergeNewer(page)
                state.isClosed = false
                publish()
                guard page.count >= Self.pageSize, let newest = state.timeline.newestID, newest != cursor else {
                    return
                }
                cursor = newest
            } catch {
                fail(error, firstLoad: false)
                publish()
                return
            }
        }
    }

    private func deliver(key: String) async {
        guard let send = state.timeline.unsent(key: key), let submission = submissions[key] else { return }
        let cabalID = cabalID
        let body = send.body
        do {
            let stored = try await api.submit(submission, payload: body, operation: "postChatMessage:\(cabalID)") {
                client, idempotencyKey in
                try await client.postChatMessage(
                    path: .init(id: cabalID),
                    headers: .init(idempotencyKey: idempotencyKey),
                    body: .json(.init(body: body))
                ).created.body.json
            }
            submissions[key] = nil
            state.timeline.settle(stored, key: key)
        } catch {
            let failure = APIError(error)
            if Self.isClosed(failure) {
                submissions[key] = nil
                state.isClosed = true
                state.timeline.dropUnsent(key: key)
            } else {
                state.timeline.setFailed(key: key, true)
                notify(failure)
            }
        }
        publish()
    }

    private func fetch(before: String? = nil, after: String? = nil) async throws -> [ChatMessage] {
        let cabalID = cabalID
        return try await api.read { client in
            try await client.getChatMessages(
                path: .init(id: cabalID),
                query: .init(before: before, after: after, limit: Self.pageSize)
            ).ok.body.json.messages
        }
    }

    private func fail(_ error: any Error, firstLoad: Bool) {
        let failure = APIError(error)
        let closed = Self.isClosed(failure)
        if closed { state.isClosed = true }
        if firstLoad {
            state.load = .failed(failure)
        } else if !closed {
            notify(failure)
        }
    }

    private func notify(_ error: APIError) {
        noticeSerial += 1
        state.notice = Notice(serial: noticeSerial, error: error)
    }

    func publish() {
        for continuation in subscribers.values { continuation.yield(state) }
    }

    private func removeSubscriber(_ id: UUID) {
        subscribers[id] = nil
    }

    static func isClosed(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error else { return false }
        return problem.code == .known(.notCabalMember)
    }
}
