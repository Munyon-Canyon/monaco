import Foundation
import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import OpenAPIRuntime
import XCTest

enum CommentsTestSupport {
    typealias Reply = StubTransport.Reply
    typealias Thread = Components.Schemas.CommentThread

    static var pendingProblem: Components.Schemas.Problem {
        Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Error", status: 503, code: .feedItemPending,
            message: "Still arriving.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: true)
    }

    static func api(_ transport: any ClientTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    static func idempotencyKey(of request: HTTPRequest) throws -> String? {
        let name = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        return request.headerFields[name]
    }

    @MainActor static func waitUntil(_ predicate: @escaping @MainActor () async -> Bool) async -> Bool {
        let deadline = ContinuousClock.now.advanced(by: .seconds(5))
        while ContinuousClock.now < deadline {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    static func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    static func page(_ threads: [Thread], feedObjectID: String? = nil) throws -> Reply {
        let json =
            if let feedObjectID {
                try encode(
                    Components.Schemas.ProposalCommentPage(feedObjectId: feedObjectID, items: threads, nextCursor: nil))
            } else {
                try encode(Components.Schemas.CommentPage(items: threads, nextCursor: nil))
            }
        return .json(.ok, json)
    }

    static func detail(canComment: Bool) throws -> Reply {
        .json(
            .ok,
            try encode(
                Components.Schemas.FeedItemDetail(
                    item: Components.Schemas.FeedItem.samples[0], visible: true, canComment: canComment)))
    }
}

struct NoRoute: Error { let path: String }

actor PathRoutedTransport: ClientTransport {
    private var routes: [String: [CommentsTestSupport.Reply]]
    private(set) var sent: [HTTPRequest] = []

    init(_ routes: [String: [CommentsTestSupport.Reply]]) {
        self.routes = routes
    }

    func send(
        _ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        sent.append(request)
        let path = request.path ?? ""
        guard var queue = routes[path], !queue.isEmpty else { throw NoRoute(path: path) }
        let reply = queue.removeFirst()
        routes[path] = queue
        switch reply {
        case .response(let response, let body): return (response, HTTPBody(body))
        case .failure(let error): throw error
        case .hang, .gate: throw NoRoute(path: path)
        }
    }
}
