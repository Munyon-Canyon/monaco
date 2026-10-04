import Foundation
import HTTPTypes
import OpenAPIRuntime

/// An RFC 9457 problem the backend answered with. Show `message`; switch on `code` only
/// where a flow branches.
public struct ProblemError: Error, Sendable, Hashable, Decodable, LocalizedError {
    public enum Code: Sendable, Hashable, Decodable {
        case known(Components.Schemas.ErrorCode)
        /// A code added to the backend after this build shipped.
        case unrecognized(String)

        public init(_ wire: String) {
            self = Components.Schemas.ErrorCode(rawValue: wire).map(Code.known) ?? .unrecognized(wire)
        }

        public init(from decoder: any Decoder) throws {
            self.init(try decoder.singleValueContainer().decode(String.self))
        }

        /// The code as the backend sent it.
        public var wire: String {
            switch self {
            case .known(let code): code.rawValue
            case .unrecognized(let wire): wire
            }
        }
    }

    public let status: Int
    public let code: Code
    public let message: String
    public let traceID: String
    public let retryable: Bool
    public internal(set) var retryAfterSeconds: Int? = nil

    public var errorDescription: String? { message }

    public init(status: Int, code: Code, message: String, traceID: String, retryable: Bool) {
        self.status = status
        self.code = code
        self.message = message
        self.traceID = traceID
        self.retryable = retryable
    }

    public init(_ problem: Components.Schemas.Problem) {
        self.init(
            status: problem.status,
            code: .known(problem.code),
            message: problem.message,
            traceID: problem.traceId,
            retryable: problem.retryable
        )
    }

    /// The problem behind an error a generated client call threw, if the backend sent one.
    public init?(_ error: any Error) {
        switch error {
        case let problem as ProblemError:
            self = problem
        case let clientError as ClientError:
            guard let problem = clientError.underlyingError as? ProblemError else { return nil }
            self = problem
        default:
            return nil
        }
    }

    private enum CodingKeys: String, CodingKey {
        case status, code, message, retryable
        case traceID = "trace_id"
    }
}

struct ProblemMiddleware: ClientMiddleware {
    private static let maxBodyBytes = 64 * 1024

    func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let (response, responseBody) = try await next(request, body, baseURL)
        guard response.status.code >= 400, let responseBody, Self.isProblem(response) else {
            return (response, responseBody)
        }
        let data = try await Data(collecting: responseBody, upTo: Self.maxBodyBytes)
        var problem = try JSONDecoder().decode(ProblemError.self, from: data)
        problem.retryAfterSeconds = response.headerFields[.retryAfter].flatMap { Int($0) }
        throw problem
    }

    private static func isProblem(_ response: HTTPResponse) -> Bool {
        guard let contentType = response.headerFields[.contentType] else { return false }
        let mediaType = contentType.split(separator: ";", maxSplits: 1, omittingEmptySubsequences: false)[0]
        return mediaType.trimmingCharacters(in: .whitespaces).lowercased() == "application/problem+json"
    }
}
