import Foundation
import MonacoAPI

public actor ThreadSession {
    public struct State: Equatable, Sendable {
        public var load: ChatLoad = .idle
        public var parent: ChatMessage?
        public var timeline: ChatTimeline
        public var isClosed = false
        public var isLoadingOlder = false
        public var notice: ChatSession.Notice?
    }

    public nonisolated let parentID: String
    private let cabalID: String
    private let chat: ChatSession
    private let api: APIClient
    private let now: @Sendable () -> Date
    private var state: State
    private var subscribers: [UUID: AsyncStream<State>.Continuation] = [:]
    private var submissions: [String: IdempotentSubmission] = [:]
    private var noticeSerial = 0
    private var isOpen = false

    init(
        parentID: String,
        viewerID: String,
        chat: ChatSession,
        api: APIClient,
        now: @escaping @Sendable () -> Date
    ) {
        self.parentID = parentID
        self.cabalID = chat.cabalID
        self.chat = chat
        self.api = api
        self.now = now
        self.state = State(timeline: ChatTimeline(viewerID: viewerID, scope: .thread(parentID: parentID)))
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
        await chat.attach(self)
        await loadNewest()
    }

    public func close() async {
        guard isOpen else { return }
        isOpen = false
        await chat.detach(self)
    }

    public func reload() async {
        await loadNewest()
    }

    public func loadOlder() async {
        guard let cursor = state.timeline.oldestID, state.timeline.hasOlder, !state.isLoadingOlder else { return }
        state.isLoadingOlder = true
        publish()
        do {
            let thread = try await fetch(before: cursor)
            state.timeline.mergeOlder(thread.replies, pageSize: ChatSession.pageSize)
        } catch {
            fail(error, firstLoad: false)
        }
        state.isLoadingOlder = false
        publish()
    }

    @discardableResult
    public func send(body: String, alsoInChannel: Bool = false) async -> GroupChatDraft.Problem? {
        let trimmed: String
        switch GroupChatDraft.validate(body) {
        case .success(let text): trimmed = text
        case .failure(let problem): return problem
        }
        let key = await chat.newKey()
        submissions[key] = IdempotentSubmission(makeKey: { key })
        state.timeline.addUnsent(
            key: key, body: trimmed, at: now(), parentID: parentID, alsoInChannel: alsoInChannel)
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
        switch event {
        case .messageCreated(let message):
            guard state.timeline.insertLive(message) else { return }
        case .threadUpdated(let id, let replyCount, let lastReplyAt):
            guard id == parentID else { return }
            state.parent?.replyCount = replyCount
            state.parent?.lastReplyAt = lastReplyAt
        case .messageDeleted(let id):
            guard id != parentID else {
                state.parent?.deleted = true
                state.parent?.body = nil
                break
            }
            state.timeline.markDeleted(id: id)
        case .attached(let resumed):
            if !resumed { await loadNewest() }
            return
        case .seenUpdated, .detached:
            return
        }
        publish()
    }

    func hide(id: String) -> ChatMessage? {
        let previous: ChatMessage?
        if id == parentID {
            previous = state.parent
            state.parent?.deleted = true
            state.parent?.body = nil
        } else {
            previous = state.timeline.message(id: id)
            state.timeline.markDeleted(id: id)
        }
        if previous != nil { publish() }
        return previous
    }

    func restore(_ message: ChatMessage) {
        if message.id == parentID {
            state.parent = message
        } else {
            state.timeline.restore(message)
        }
        publish()
    }

    private func loadNewest() async {
        if state.load != .loaded { state.load = .loading }
        publish()
        do {
            let thread = try await fetch()
            state.parent = thread.parent
            state.timeline.mergeNewest(thread.replies, pageSize: ChatSession.pageSize)
            state.load = .loaded
            state.isClosed = false
            await chat.applyParent(thread.parent)
        } catch {
            fail(error, firstLoad: state.parent == nil)
        }
        publish()
    }

    private func deliver(key: String) async {
        guard let send = state.timeline.unsent(key: key), let submission = submissions[key] else { return }
        let cabalID = cabalID
        let request = Components.Schemas.PostChatMessageRequest(
            body: send.body, parentId: parentID, alsoInChannel: send.alsoInChannel)
        do {
            let stored = try await api.submit(
                submission, payload: request, operation: "postChatMessage:\(cabalID):\(parentID)"
            ) { client, idempotencyKey in
                try await client.postChatMessage(
                    path: .init(id: cabalID),
                    headers: .init(idempotencyKey: idempotencyKey),
                    body: .json(request)
                ).created.body.json
            }
            submissions[key] = nil
            state.timeline.settle(stored, key: key)
            publish()
            if send.alsoInChannel { await chat.receiveReply(stored) }
            await loadNewest()
        } catch {
            let failure = APIError(error)
            if ChatSession.isClosed(failure) {
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

    private func fetch(before: String? = nil) async throws -> Components.Schemas.ChatThread {
        let cabalID = cabalID
        let parentID = parentID
        return try await api.read { client in
            try await client.getChatThread(
                path: .init(id: cabalID, messageId: parentID),
                query: .init(before: before, limit: ChatSession.pageSize)
            ).ok.body.json
        }
    }

    private func fail(_ error: any Error, firstLoad: Bool) {
        let failure = APIError(error)
        let closed = ChatSession.isClosed(failure)
        if closed { state.isClosed = true }
        if firstLoad {
            state.load = .failed(failure)
        } else if !closed {
            notify(failure)
        }
    }

    private func notify(_ error: APIError) {
        noticeSerial += 1
        state.notice = ChatSession.Notice(serial: noticeSerial, error: error)
    }

    private func publish() {
        for continuation in subscribers.values { continuation.yield(state) }
    }

    private func removeSubscriber(_ id: UUID) {
        subscribers[id] = nil
    }
}
