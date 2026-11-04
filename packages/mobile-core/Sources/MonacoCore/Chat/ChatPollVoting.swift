import Foundation
import MonacoAPI

public struct ChatPollCount: Sendable, Equatable, Decodable {
    public let position: Int
    public let votes: Int

    public init(position: Int, votes: Int) {
        self.position = position
        self.votes = votes
    }
}

public struct ChatPollVoting: PollVoting {
    private let cabalID: String
    private let api: APIClient
    private let settled: @Sendable (String, Components.Schemas.PollState) async -> Void
    private let messageGone: @Sendable (String) async -> Void

    public init(
        cabalID: String, api: APIClient,
        settled: @escaping @Sendable (String, Components.Schemas.PollState) async -> Void = { _, _ in },
        messageGone: @escaping @Sendable (String) async -> Void = { _ in }
    ) {
        self.cabalID = cabalID
        self.api = api
        self.settled = settled
        self.messageGone = messageGone
    }

    public func vote(commentID: String, position: Int) async throws -> Components.Schemas.PollState {
        let cabalID = cabalID
        return try await guarded(commentID) {
            try await api.submit(
                IdempotentSubmission(), payload: "\(cabalID)|\(commentID)|\(position)", operation: "voteChatPoll"
            ) { client, key in
                try await client.voteChatPoll(
                    path: .init(id: cabalID, messageId: commentID), headers: .init(idempotencyKey: key),
                    body: .json(.init(position: position))
                ).ok.body.json
            }
        }
    }

    public func withdraw(commentID: String) async throws -> Components.Schemas.PollState {
        let cabalID = cabalID
        return try await guarded(commentID) {
            try await api.submit(
                IdempotentSubmission(), payload: "\(cabalID)|\(commentID)", operation: "unvoteChatPoll"
            ) { client, key in
                try await client.unvoteChatPoll(
                    path: .init(id: cabalID, messageId: commentID), headers: .init(idempotencyKey: key)
                ).ok.body.json
            }
        }
    }

    private func guarded(
        _ messageID: String, _ call: () async throws -> Components.Schemas.PollState
    ) async throws -> Components.Schemas.PollState {
        do {
            let state = try await call()
            await settled(messageID, state)
            return state
        } catch {
            if Self.isGone(APIError(error)) { await messageGone(messageID) }
            throw error
        }
    }

    static func isGone(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error else { return false }
        return problem.code == .known(.chatMessageNotFound)
    }
}

extension ChatMessage {
    mutating func settlePoll(_ state: Components.Schemas.PollState) -> Bool {
        guard case .poll(var poll) = attachment else { return false }
        poll.closesAt = state.closesAt
        poll.closed = state.closed
        poll.options = state.options
        poll.total = state.total
        poll.myPosition = state.myPosition
        attachment = .poll(poll)
        return true
    }

    mutating func applyPoll(counts: [ChatPollCount], total: Int) -> Bool {
        guard case .poll(var poll) = attachment else { return false }
        let votes = Dictionary(counts.map { ($0.position, $0.votes) }, uniquingKeysWith: { _, last in last })
        for index in poll.options.indices {
            poll.options[index].count = votes[poll.options[index].position] ?? 0
        }
        poll.total = total
        attachment = .poll(poll)
        return true
    }
}

extension ChatSession {
    public func settlePoll(id: String, state pollState: Components.Schemas.PollState) {
        guard state.timeline.settlePoll(id: id, state: pollState) else { return }
        publish()
    }

    public func dropMessage(id: String) {
        state.timeline.markDeleted(id: id)
        publish()
    }

    public nonisolated var pollVoting: ChatPollVoting {
        ChatPollVoting(
            cabalID: cabalID, api: api,
            settled: { [self] id, state in await settlePoll(id: id, state: state) },
            messageGone: { [self] id in await dropMessage(id: id) })
    }
}

extension ChatTimeline {
    @discardableResult
    public mutating func applyPoll(id: String, counts: [ChatPollCount], total: Int) -> Bool {
        updateMessage(id: id) { $0.applyPoll(counts: counts, total: total) }
    }

    @discardableResult
    public mutating func settlePoll(id: String, state: Components.Schemas.PollState) -> Bool {
        updateMessage(id: id) { $0.settlePoll(state) }
    }
}
