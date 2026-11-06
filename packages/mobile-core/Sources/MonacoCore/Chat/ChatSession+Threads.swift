import Foundation
import MonacoAPI

extension ChatSession {
    public func thread(parentId: String) -> ThreadSession {
        if let existing = threads[parentId]?.session { return existing }
        let created = ThreadSession(
            parentID: parentId, viewerID: state.timeline.viewerID, chat: self, api: api, now: now)
        threads[parentId] = WeakThread(session: created)
        return created
    }

    public func delete(messageId: String) async -> APIError? {
        let commentID = await commentID(of: messageId)
        let targetID = commentID ?? messageId
        let channelBefore = state.timeline.messages(matching: targetID)
        state.timeline.markDeleted(id: targetID)
        await state.pins.drop(messageID: messageId)
        var threadsBefore: [(ThreadSession, [ChatMessage])] = []
        for thread in attached.values {
            let previous = await thread.hide(id: targetID)
            if !previous.isEmpty { threadsBefore.append((thread, previous)) }
        }
        publish()
        let submission = deletions[targetID] ?? IdempotentSubmission(makeKey: makeKey)
        deletions[targetID] = submission
        let cabalID = cabalID
        do {
            if let commentID {
                _ = try await api.submit(submission, payload: commentID, operation: "deleteFeedComment") {
                    client, idempotencyKey in
                    try await client.deleteFeedComment(
                        path: .init(commentId: commentID), headers: .init(idempotencyKey: idempotencyKey)
                    ).noContent
                }
            } else {
                _ = try await api.submit(submission, payload: messageId, operation: "deleteChatMessage:\(cabalID)") {
                    client, idempotencyKey in
                    try await client.deleteChatMessage(
                        path: .init(id: cabalID, messageId: messageId),
                        headers: .init(idempotencyKey: idempotencyKey)
                    ).noContent
                }
            }
            deletions[targetID] = nil
            return nil
        } catch {
            for message in channelBefore { state.timeline.restore(message) }
            for (thread, previous) in threadsBefore { await thread.restore(previous) }
            publish()
            return APIError(error)
        }
    }

    private func commentID(of messageId: String) async -> String? {
        if let id = state.timeline.message(id: messageId)?.commentID { return id }
        for thread in attached.values {
            if let id = await thread.commentID(of: messageId) { return id }
        }
        return nil
    }

    func unblocked(_ event: ChatRealtimeEvent) async -> ChatRealtimeEvent {
        guard case .threadUpdated(let id, let replyCount, let lastReplyAt, let lastRepliers?) = event,
            let blocked
        else { return event }
        let authors = await blocked.authors
        return .threadUpdated(
            id: id, replyCount: replyCount, lastReplyAt: lastReplyAt,
            lastRepliers: lastRepliers.filter { !authors.contains($0.id) })
    }

    func attach(_ thread: ThreadSession) {
        attached[thread.parentID] = thread
        subscribe()
    }

    func detach(_ thread: ThreadSession) {
        guard attached[thread.parentID] === thread else { return }
        attached[thread.parentID] = nil
        if !isOpen && attached.isEmpty { stopListening() }
    }

    func receiveReply(_ message: ChatMessage) {
        guard state.timeline.hasLoadedNewest, state.timeline.insertLive(message) else { return }
        publish()
    }

    func applyParent(_ parent: ChatMessage) {
        state.timeline.applyThread(
            id: parent.id, replyCount: parent.replyCount, lastReplyAt: parent.lastReplyAt,
            lastRepliers: parent.lastRepliers)
        publish()
    }
}

struct WeakThread {
    weak var session: ThreadSession?
}
