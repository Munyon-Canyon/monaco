import Foundation
import HTTPTypes
import MonacoAPI
import OpenAPIRuntime

public let testServerURL = URL(string: "http://api.test")!

/// A `ClientTransport` that never touches the network. It answers every request with one
/// fixed reply, or, when scripted, with the next reply in its queue.
public actor StubTransport: ClientTransport {
    public enum Reply: Sendable {
        case response(HTTPResponse, Data)
        case failure(any Error)
        /// Never answers; the request ends only when its task is cancelled.
        case hang
        case gate

        public static func ok(_ text: String) -> Reply {
            .response(status: .ok, contentType: "text/plain", body: Data(text.utf8))
        }

        public static func json(_ status: HTTPResponse.Status, _ body: String) -> Reply {
            .response(status: status, contentType: "application/json", body: Data(body.utf8))
        }

        public static func problem(_ problem: Components.Schemas.Problem) throws -> Reply {
            .response(
                status: .init(code: problem.status),
                contentType: "application/problem+json; charset=utf-8",
                body: try JSONEncoder().encode(problem)
            )
        }

        public static func response(status: HTTPResponse.Status, contentType: String, body: Data) -> Reply {
            var response = HTTPResponse(status: status)
            response.headerFields[.contentType] = contentType
            return .response(response, body)
        }
    }

    /// Thrown when a scripted transport receives more requests than it has replies.
    public struct ScriptExhausted: Error {
        public let request: Int
    }

    private enum Mode {
        case fixed(Reply)
        case scripted([Reply])
    }

    private var mode: Mode
    private var gates: [CheckedContinuation<(HTTPResponse, HTTPBody?), Error>] = []
    private var waiters: [(count: Int, continuation: CheckedContinuation<Void, Never>)] = []
    public private(set) var sent: [HTTPRequest] = []
    public private(set) var sentBodies: [Data?] = []

    public init(_ reply: Reply) {
        mode = .fixed(reply)
    }

    public init(status: HTTPResponse.Status, contentType: String, body: Data) {
        self.init(.response(status: status, contentType: contentType, body: body))
    }

    /// Answers the first request with `replies[0]`, the second with `replies[1]`, and so on.
    public init(scripted replies: [Reply]) {
        mode = .scripted(replies)
    }

    public func send(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        sent.append(request)
        if let body {
            sentBodies.append(try await Data(collecting: body, upTo: 64 * 1024))
        } else {
            sentBodies.append(nil)
        }
        let ready = waiters.filter { $0.count <= sent.count }
        waiters.removeAll { $0.count <= sent.count }
        for waiter in ready { waiter.continuation.resume() }
        switch try nextReply() {
        case .response(let response, let body):
            return (response, HTTPBody(body))
        case .failure(let error):
            throw error
        case .hang:
            while true {
                try await Task.sleep(nanoseconds: 86_400_000_000_000)
            }
        case .gate:
            return try await withCheckedThrowingContinuation { continuation in
                gates.append(continuation)
            }
        }
    }

    public func waitForRequest() async {
        await waitForRequests(1)
    }

    public func waitForRequests(_ count: Int) async {
        if sent.count >= count { return }
        await withCheckedContinuation { waiters.append((count, $0)) }
    }

    public func releaseGate(_ reply: Reply) {
        let continuation = gates.removeFirst()
        switch reply {
        case .response(let response, let body):
            continuation.resume(returning: (response, HTTPBody(body)))
        case .failure(let error):
            continuation.resume(throwing: error)
        case .hang, .gate:
            continuation.resume(throwing: ScriptExhausted(request: sent.count))
        }
    }

    private func nextReply() throws -> Reply {
        switch mode {
        case .fixed(let reply):
            return reply
        case .scripted(var replies):
            guard !replies.isEmpty else { throw ScriptExhausted(request: sent.count) }
            let reply = replies.removeFirst()
            mode = .scripted(replies)
            return reply
        }
    }

    public static func ok(_ text: String) -> StubTransport {
        StubTransport(.ok(text))
    }

    public static func problem(_ problem: Components.Schemas.Problem) throws -> StubTransport {
        StubTransport(try .problem(problem))
    }
}
