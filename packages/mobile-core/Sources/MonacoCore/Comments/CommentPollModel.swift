import Foundation
import MonacoAPI
import Observation

public protocol PollVoting: Sendable {
    func vote(commentID: String, position: Int) async throws -> Components.Schemas.PollState
    func withdraw(commentID: String) async throws -> Components.Schemas.PollState
}

public struct LivePollVoting: PollVoting {
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func vote(commentID: String, position: Int) async throws -> Components.Schemas.PollState {
        try await api.submit(
            IdempotentSubmission(), payload: "\(commentID)|\(position)", operation: "putCommentPollVote"
        ) { client, key in
            try await client.putCommentPollVote(
                path: .init(commentId: commentID), headers: .init(idempotencyKey: key),
                body: .json(.init(position: position))
            ).ok.body.json
        }
    }

    public func withdraw(commentID: String) async throws -> Components.Schemas.PollState {
        try await api.submit(IdempotentSubmission(), payload: commentID, operation: "deleteCommentPollVote") {
            client, key in
            try await client.deleteCommentPollVote(
                path: .init(commentId: commentID), headers: .init(idempotencyKey: key)
            ).ok.body.json
        }
    }
}

@Observable
@MainActor
public final class CommentPollModel {
    public static let tick: Duration = .seconds(30)

    public private(set) var poll: Components.Schemas.PollAttachment
    public private(set) var failure: APIError?
    public private(set) var noticeTick = 0
    public private(set) var isSending = false

    private let commentID: String
    private let voting: any PollVoting
    private let clock: any Clock<Duration>
    private var untilClose: Duration
    @ObservationIgnored private var waited: Duration = .zero

    public init(
        commentID: String, poll: Components.Schemas.PollAttachment, now: Date, voting: any PollVoting,
        clock: any Clock<Duration>
    ) {
        self.commentID = commentID
        self.poll = poll
        self.voting = voting
        self.clock = clock
        untilClose = .seconds(max(poll.closesAt.timeIntervalSince(now), 0))
    }

    public var isClosed: Bool { poll.closed || untilClose <= .zero }

    public var myPosition: Int? { poll.myPosition }

    public func shareBps(_ option: Components.Schemas.PollOption) -> Int {
        guard poll.total > 0 else { return 0 }
        return option.count * 10_000 / poll.total
    }

    public func percent(_ option: Components.Schemas.PollOption) -> Int {
        (shareBps(option) + 50) / 100
    }

    public var footer: String {
        let votes = CommentsCopy.pollVotes(poll.total)
        guard !isClosed else { return CommentsCopy.pollClosed }
        return "\(votes) · closes in \(Self.countdown(untilClose))"
    }

    public func reconcile(_ next: Components.Schemas.PollAttachment, now: Date) {
        guard !isSending else { return }
        poll = next
        untilClose = .seconds(max(next.closesAt.timeIntervalSince(now), 0))
    }

    public func run() async {
        while untilClose > .zero {
            let step = min(Self.tick, untilClose)
            do { try await clock.sleep(for: step) } catch { return }
            untilClose -= step
        }
    }

    public func tap(_ position: Int) async {
        guard !isClosed, !isSending, poll.options.contains(where: { $0.position == position }) else { return }
        let before = poll
        let withdrawing = poll.myPosition == position
        poll = Self.applying(position: withdrawing ? nil : position, to: poll)
        isSending = true
        defer { isSending = false }
        do {
            let state: Components.Schemas.PollState
            if withdrawing {
                state = try await voting.withdraw(commentID: commentID)
            } else {
                state = try await voting.vote(commentID: commentID, position: position)
            }
            poll.closesAt = state.closesAt
            poll.closed = state.closed
            poll.options = state.options
            poll.total = state.total
            poll.myPosition = state.myPosition
        } catch {
            poll = before
            failure = APIError(error)
            noticeTick += 1
        }
    }

    public nonisolated static func applying(
        position: Int?, to poll: Components.Schemas.PollAttachment
    ) -> Components.Schemas.PollAttachment {
        var next = poll
        for index in next.options.indices {
            if next.options[index].position == poll.myPosition { next.options[index].count -= 1 }
            if next.options[index].position == position { next.options[index].count += 1 }
        }
        next.total = next.options.reduce(0) { $0 + $1.count }
        next.myPosition = position
        return next
    }

    public nonisolated static func countdown(_ remaining: Duration) -> String {
        let minutes = Int(remaining.components.seconds / 60)
        if minutes >= 2880 { return "\(minutes / 1440)d" }
        if minutes >= 60 { return "\(minutes / 60)h" }
        return "\(max(minutes, 1))m"
    }
}

extension CommentsCopy {
    public static let pollClosed = "Closed"

    public static func pollVotes(_ total: Int) -> String { total == 1 ? "1 vote" : "\(total) votes" }
}
