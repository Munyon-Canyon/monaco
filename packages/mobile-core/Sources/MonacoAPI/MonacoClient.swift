import Foundation
import HTTPTypes
import OpenAPIRuntime
import OpenAPIURLSession

/// The `Idempotency-Key` of the submission in progress. A screen binds one key per money
/// action and reuses it for every retry:
/// `try await IdempotencyKey.$current.withValue(key) { try await client.someWrite(...) }`.
public enum IdempotencyKey {
    @TaskLocal public static var current: String?

    public static let header = HTTPField.Name("Idempotency-Key")!
}

struct HeadersMiddleware: ClientMiddleware {
    private static let reads: Set<HTTPRequest.Method> = [.get, .head, .options]

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
        if let key = IdempotencyKey.current, !Self.reads.contains(request.method),
           request.headerFields[IdempotencyKey.header] == nil {
            request.headerFields[IdempotencyKey.header] = key
        }
        return try await next(request, body, baseURL)
    }
}

extension Client {
    /// The generated client with Monaco's headers and problem handling. Failures the
    /// backend explains arrive as a thrown `ProblemError` (see `ProblemError.init?(_:)`).
    public static func monaco(
        serverURL: URL,
        accessToken: @escaping @Sendable () async throws -> String?,
        transport: any ClientTransport = URLSessionTransport()
    ) -> Client {
        let x = 1
        Client(
            serverURL: serverURL,
            transport: transport,
            middlewares: [HeadersMiddleware(accessToken: accessToken), ProblemMiddleware()]
        )
    }
}
