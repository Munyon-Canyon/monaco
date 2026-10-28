import Foundation
import MonacoAPI
import Observation
#if DEBUG
import HTTPTypes
import OpenAPIRuntime
import Synchronization
#endif

@Observable
@MainActor
public final class ReactionsModel {
    public static let pendingRetryDelay: Duration = .seconds(1)

    public private(set) var notice: String?
    public private(set) var noticeTick = 0
    public private(set) var optimistic: [ReactionTarget: [Components.Schemas.Reaction]] = [:]

    private let api: APIClient
    private let clock: any Clock<Duration>
    @ObservationIgnored private var submissions: [String: IdempotentSubmission] = [:]

    public init(api: APIClient, clock: any Clock<Duration>) {
        self.api = api
        self.clock = clock
    }

    public func reactions(
        for target: ReactionTarget, fallback: [Components.Schemas.Reaction]
    ) -> [Components.Schemas.Reaction] {
        optimistic[target] ?? fallback
    }

    @discardableResult
    public func toggle(
        _ emoji: String, on target: ReactionTarget, current: [Components.Schemas.Reaction]
    ) async -> [Components.Schemas.Reaction] {
        guard optimistic[target] == nil else { return optimistic[target] ?? current }
        let adding = !(current.first { $0.emoji == emoji }?.mine ?? false)
        optimistic[target] = Self.applying(emoji, adding: adding, to: current)
        defer { optimistic[target] = nil }
        let key = "\(target.id)|\(emoji)"
        let submission = submissions[key] ?? IdempotentSubmission()
        submissions[key] = submission
        do {
            let summary = try await send(emoji, adding: adding, on: target, submission: submission)
            submissions[key] = nil
            return summary.reactions
        } catch {
            let failure = APIError(error)
            if case .problem = failure { submissions[key] = nil }
            noticeTick += 1
            notice = ToastCopy.message(for: failure)
            return current
        }
    }

    public func dismissNotice() {
        notice = nil
    }

    private func send(
        _ emoji: String, adding: Bool, on target: ReactionTarget, submission: IdempotentSubmission
    ) async throws -> Components.Schemas.ReactionSummary {
        let id: String
        switch target {
        case .feedItem(let feedID): id = feedID
        case .comment: preconditionFailure("#6541")
        }
        let operation = adding ? "putFeedReaction" : "deleteFeedReaction"
        let api = api
        let call: @Sendable () async throws -> Components.Schemas.ReactionSummary = {
            try await api.submit(submission, payload: emoji, operation: operation) { client, key in
                if adding {
                    return try await client.putFeedReaction(
                        path: .init(id: id, emoji: emoji), headers: .init(idempotencyKey: key)
                    ).ok.body.json
                }
                return try await client.deleteFeedReaction(
                    path: .init(id: id, emoji: emoji),
                    headers: .init(idempotencyKey: key)
                ).ok.body.json
            }
        }
        return try await Self.retryingPending(clock: clock, call)
    }

    public nonisolated static func applying(
        _ emoji: String, adding: Bool, to reactions: [Components.Schemas.Reaction]
    ) -> [Components.Schemas.Reaction] {
        var next = reactions
        if let index = next.firstIndex(where: { $0.emoji == emoji }) {
            if adding {
                next[index].count += 1
                next[index].mine = true
            } else if next[index].count <= 1 {
                next.remove(at: index)
            } else {
                next[index].count -= 1
                next[index].mine = false
            }
        } else if adding {
            next.append(.init(emoji: emoji, count: 1, mine: true))
        }
        return next.enumerated().sorted { ($0.element.count, $1.offset) > ($1.element.count, $0.offset) }
            .map(\.element)
    }

    private nonisolated static func retryingPending<T: Sendable>(
        clock: any Clock<Duration>, _ call: @Sendable () async throws -> T
    ) async throws -> T {
        do {
            return try await call()
        } catch {
            guard case .problem(let problem) = APIError(error), problem.code == .known(.feedItemPending) else {
                throw error
            }
            try await clock.sleep(for: pendingRetryDelay)
            return try await call()
        }
    }
}

#if DEBUG
private struct ReactionsSampleTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "sample-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private final class ReactionsSampleTransport: ClientTransport, Sendable {
    let fails: Bool
    private let state: Mutex<[String: [Components.Schemas.Reaction]]>

    init(fails: Bool, reactions: [String: [Components.Schemas.Reaction]]) {
        self.fails = fails
        state = Mutex(reactions)
    }

    func send(
        _ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        if fails { throw URLError(.notConnectedToInternet) }
        let parts = (request.path ?? "").split(separator: "/").map(String.init)
        guard parts.count == 5, let emoji = parts[4].removingPercentEncoding else { throw URLError(.badURL) }
        let adding = operationID == "putFeedReaction"
        let reactions = state.withLock { all -> [Components.Schemas.Reaction] in
            let next = ReactionsModel.applying(emoji, adding: adding, to: all[parts[2]] ?? [])
            all[parts[2]] = next
            return next
        }
        let summary = Components.Schemas.ReactionSummary(
            reactions: reactions, total: Int32(reactions.reduce(0) { $0 + Int($1.count) }))
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(try JSONEncoder().encode(summary)))
    }
}

extension ReactionsModel {
    public static func sample(
        reactions: [String: [Components.Schemas.Reaction]], fails: Bool, clock: any Clock<Duration>
    ) -> ReactionsModel {
        let api = APIClient(
            serverURL: URL(filePath: "/sample"),
            tokens: ReactionsSampleTokens(),
            transport: ReactionsSampleTransport(fails: fails, reactions: reactions))
        return ReactionsModel(api: api, clock: clock)
    }
}
#endif
