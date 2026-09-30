import Foundation
import HTTPTypes
import OpenAPIRuntime

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

struct HeadersMiddleware: ClientMiddleware {
    let accessToken: @Sendable () async throws -> String?

    func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var request = request
        if let token = try await accessToken() {
            request.headerFields[.authorization] = "Bearer \(token)"
        }
        return try await next(request, body, baseURL)
    }
}

/// Answers a 401 by asking for a fresh token once and resending. It sits inside
/// `ProblemMiddleware`, so it sees the 401 as a response rather than a thrown problem.
struct RefreshMiddleware: ClientMiddleware {
    private static let bearerPrefix = "Bearer "

    let tokens: any AccessTokenProvider

    func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let (response, responseBody) = try await next(request, body, baseURL)
        guard response.status == .unauthorized else { return (response, responseBody) }
        guard let sent = request.headerFields[.authorization], sent.hasPrefix(Self.bearerPrefix),
            let fresh = try await tokens.refreshedToken(replacing: String(sent.dropFirst(Self.bearerPrefix.count)))
        else {
            throw APIError.signedOut
        }
        var retry = request
        retry.headerFields[.authorization] = Self.bearerPrefix + fresh
        let (retried, retriedBody) = try await next(retry, body, baseURL)
        guard retried.status != .unauthorized else { throw APIError.signedOut }
        return (retried, retriedBody)
    }
}

/// Gives up on a request whose response has not started within its budget and throws
/// `URLError(.timedOut)`. The budgets match `MonacoRequestTimeout.standard` and
/// `.moneyWrite`: a request carrying an `Idempotency-Key` is a money write the backend
/// confirms on chain inside the request, so it gets room, and a retry after the deadline
/// replays under the same key.
struct TimeoutMiddleware: ClientMiddleware {
    static let read = Duration.seconds(15)
    static let keyedWrite = Duration.seconds(60)
    private static let keyHeader = HTTPField.Name(IdempotentSubmission.keyHeader)!

    private let sleep: @Sendable (Duration) async throws -> Void

    init(clock: some Clock<Duration>) {
        sleep = { try await clock.sleep(for: $0) }
    }

    static func budget(for request: HTTPRequest) -> Duration {
        request.headerFields[keyHeader] == nil ? read : keyedWrite
    }

    func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let budget = Self.budget(for: request)
        return try await withoutActuallyEscaping(next) { next in
            try await withThrowingTaskGroup(of: (HTTPResponse, HTTPBody?).self) { group in
                group.addTask { try await next(request, body, baseURL) }
                group.addTask { [sleep] in
                    try await sleep(budget)
                    throw URLError(.timedOut)
                }
                defer { group.cancelAll() }
                return try await group.next()!
            }
        }
    }
}
