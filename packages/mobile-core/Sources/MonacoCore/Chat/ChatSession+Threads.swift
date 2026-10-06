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
        let channelBefore = state.timeline.message(id: messageId)
        state.timeline.markDeleted(id: messageId)
        var threadsBefore: [(ThreadSession, ChatMessage)] = []
        for thread in attached.values {
            if let previous = await thread.hide(id: messageId) { threadsBefore.append((thread, previous)) }
        }
        publish()
        let submission = deletions[messageId] ?? IdempotentSubmission(makeKey: makeKey)
        deletions[messageId] = submission
        let cabalID = cabalID
        do {
            _ = try await api.submit(submission, payload: messageId, operation: "deleteChatMessage:\(cabalID)") {
                client, idempotencyKey in
                try await client.deleteChatMessage(
                    path: .init(id: cabalID, messageId: messageId),
                    headers: .init(idempotencyKey: idempotencyKey)
                ).noContent
            }
            deletions[messageId] = nil
            return nil
        } catch {
            if let channelBefore { state.timeline.restore(channelBefore) }
            for (thread, previous) in threadsBefore { await thread.restore(previous) }
            publish()
            return APIError(error)
        }
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
        state.timeline.applyThread(id: parent.id, replyCount: parent.replyCount, lastReplyAt: parent.lastReplyAt)
        publish()
    }
}

struct WeakThread {
    weak var session: ThreadSession?
}
