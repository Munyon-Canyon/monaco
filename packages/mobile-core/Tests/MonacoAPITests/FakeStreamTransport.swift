import Foundation
import HTTPTypes
import MonacoAPI
import OpenAPIRuntime

/// A `ClientTransport` for `GET /v1/stream`. Each request takes the next scripted reply, then
/// `then` forever. A `.stream` reply stays open until the test closes it.
final class FakeStreamTransport: ClientTransport, @unchecked Sendable {
    enum Reply: Sendable {
        case stream
        case unauthorized
        case unreachable
        case hang
    }

    struct Request: Sendable {
        let at: Duration
        let authorization: String?
        let lastEventID: String?
    }

    struct Connection: Sendable {
        fileprivate let bytes: AsyncStream<ArraySlice<UInt8>>.Continuation

        func send(_ text: String) { bytes.yield(ArraySlice(text.utf8)) }
        func close() { bytes.finish() }
    }

    struct State: Sendable {
        var replies: [Reply]
        var requests: [Request] = []
        var connections: [Connection] = []
    }

    struct Unreachable: Error {}

    let state: Watched<State>
    private let then: Reply
    private let elapsed: @Sendable () -> Duration

    init(_ replies: [Reply] = [], then: Reply = .stream, clock: TestClock) {
        state = Watched(State(replies: replies))
        self.then = then
        elapsed = { clock.now.offset }
    }

    struct NeverArrived: Error {}

    func connection(_ number: Int) async throws -> Connection {
        guard await state.until({ $0.connections.count >= number }) else { throw NeverArrived() }
        return state.current.connections[number - 1]
    }

    @discardableResult
    func requests(_ count: Int) async throws -> [Request] {
        guard await state.until({ $0.requests.count >= count }) else { throw NeverArrived() }
        return state.current.requests
    }

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let (bytes, continuation) = AsyncStream.makeStream(of: ArraySlice<UInt8>.self)
        let record = Request(
            at: elapsed(),
            authorization: request.headerFields[.authorization],
            lastEventID: request.headerFields[HTTPField.Name("Last-Event-ID")!]
        )
        let reply = state.mutate { state in
            state.requests.append(record)
            let reply = state.replies.isEmpty ? then : state.replies.removeFirst()
            if case .stream = reply { state.connections.append(Connection(bytes: continuation)) }
            return reply
        }
        switch reply {
        case .stream:
            var response = HTTPResponse(status: .ok)
            response.headerFields[.contentType] = "text/event-stream"
            return (response, HTTPBody(bytes, length: .unknown, iterationBehavior: .single))
        case .unauthorized:
            let problem = Components.Schemas.Problem(
                _type: .about_colon_blank,
                title: "Unauthorized",
                status: 401,
                code: .unauthorized,
                message: "Sign in again.",
                traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
                retryable: false
            )
            var response = HTTPResponse(status: .unauthorized)
            response.headerFields[.contentType] = "application/problem+json"
            return (response, HTTPBody(try JSONEncoder().encode(problem)))
        case .unreachable:
            throw Unreachable()
        case .hang:
            try await Task.sleep(for: .seconds(3600))
            throw CancellationError()
        }
    }
}

/// Collects what one subscription receives.
final class Collector: Sendable {
    let hints = Watched<[Hint]>([])
    private let task: Task<Void, Never>

    init(_ stream: AsyncStream<Hint>) {
        task = Task { [hints] in
            for await hint in stream { hints.mutate { $0.append(hint) } }
        }
    }

    deinit { task.cancel() }

    func first(_ count: Int) async -> [Hint] {
        _ = await hints.until { $0.count >= count }
        return Array(hints.current.prefix(count))
    }
}
