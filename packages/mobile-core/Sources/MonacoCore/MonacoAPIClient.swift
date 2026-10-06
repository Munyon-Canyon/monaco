import Foundation
import MonacoAPI

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Failures the API answered with carry the `X-Request-Id` the request was sent under,
/// so an error state can show a support reference that matches the server logs.
public enum MonacoAPIError: Error, Equatable {
    case invalidResponse
    case missingAccessToken
    case httpStatus(Int, requestID: String? = nil)
    /// 4xx with a server `{"error": "..."}` message meant for the user.
    case rejected(status: Int, message: String, requestID: String? = nil)
    /// 429. `retryAfterSeconds` comes from the `Retry-After` header when present.
    case rateLimited(retryAfterSeconds: Int?, requestID: String? = nil)

    /// The status the server answered with, for callers that only care about the code.
    /// Prefer this over matching `.httpStatus`: the same status can arrive as `.rejected`
    /// or `.rateLimited` when the response carried more than a code.
    public var statusCode: Int? {
        switch self {
        case .httpStatus(let status, _), .rejected(let status, _, _):
            return status
        case .rateLimited:
            return 429
        case .invalidResponse, .missingAccessToken:
            return nil
        }
    }

    public var requestID: String? {
        switch self {
        case .httpStatus(_, let requestID),
            .rejected(_, _, let requestID),
            .rateLimited(_, let requestID):
            return requestID
        case .invalidResponse, .missingAccessToken:
            return nil
        }
    }

    /// Two errors are equal when they describe the same failure. The request id names
    /// one attempt, not the failure, so it is left out.
    public static func == (lhs: MonacoAPIError, rhs: MonacoAPIError) -> Bool {
        switch (lhs, rhs) {
        case (.invalidResponse, .invalidResponse):
            return true
        case (.missingAccessToken, .missingAccessToken):
            return true
        case (.httpStatus(let a, _), .httpStatus(let b, _)):
            return a == b
        case (.rejected(let aStatus, let aMessage, _), .rejected(let bStatus, let bMessage, _)):
            return aStatus == bStatus && aMessage == bMessage
        case (.rateLimited(let a, _), .rateLimited(let b, _)):
            return a == b
        default:
            return false
        }
    }
}

public typealias AccessTokenProvider = @Sendable () async throws -> String?

public final class MonacoAPIClient: @unchecked Sendable {
    private let baseURL: URL
    /// Every request goes through the transport so an expired access token is
    /// refreshed and the request retried once instead of surfacing a 401.
    private let session: MonacoHTTPTransport
    private let accessTokenProvider: AccessTokenProvider?

    /// - Parameter telemetry: receives one event per request; defaults to whatever is
    ///   registered in `APITelemetryRegistry.shared`.
    public convenience init(
        baseURL: URL = MonacoConfig.apiBaseURL,
        session: URLSession = .monaco,
        accessTokenProvider: AccessTokenProvider? = nil,
        telemetry: APITelemetry? = nil
    ) {
        self.init(
            baseURL: baseURL,
            transport: MonacoHTTPTransport(session: session, telemetry: telemetry),
            accessTokenProvider: accessTokenProvider
        )
    }

    public init(
        baseURL: URL = MonacoConfig.apiBaseURL,
        transport: MonacoHTTPTransport,
        accessTokenProvider: AccessTokenProvider? = nil
    ) {
        self.baseURL = baseURL
        self.session = transport
        self.accessTokenProvider = accessTokenProvider
    }

    /// How much of a failed response a route keeps.
    enum ErrorMapping {
        /// Just the status. What most routes still do, because their callers pattern-match
        /// `.httpStatus(404)` and would silently stop matching on a richer case.
        case statusOnly
        /// Everything the response carried: the `Retry-After` on a 429, and a 4xx body
        /// written for members. Routes opt in once their callers read `statusCode`.
        case full
    }

    /// The one place a response becomes an error. Returns nil when the status is accepted.
    static func error(
        for response: MonacoHTTPResponse,
        accepting: Set<Int>,
        mapping: ErrorMapping
    ) -> MonacoAPIError? {
        guard let http = response.response as? HTTPURLResponse else {
            return .invalidResponse
        }
        guard !accepting.contains(http.statusCode) else { return nil }
        let requestID = response.requestID
        guard mapping == .full else {
            return .httpStatus(http.statusCode, requestID: requestID)
        }
        if http.statusCode == 429 {
            let retryAfter = http.value(forHTTPHeaderField: "Retry-After").flatMap { Int($0) }
            return .rateLimited(retryAfterSeconds: retryAfter, requestID: requestID)
        }
        if (400..<500).contains(http.statusCode), http.statusCode != 401,
            let body = try? JSONDecoder().decode(APIErrorBody.self, from: response.data),
            !body.error.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        {
            return .rejected(status: http.statusCode, message: body.error, requestID: requestID)
        }
        return .httpStatus(http.statusCode, requestID: requestID)
    }

    /// Maps non-200 responses to `MonacoAPIError`, keeping server copy for 4xx.
    static func requireOK(_ response: MonacoHTTPResponse) throws {
        if let error = error(for: response, accepting: [200], mapping: .full) { throw error }
    }

    /// Sends `request` under its route template and returns the response when the status
    /// is in `accepting`. Any other status throws, carrying as much of the answer as
    /// `mapping` allows plus the request id.
    /// Money POSTs pass their `submission` so the idempotency key rides along.
    private func send(
        _ request: URLRequest,
        route: String,
        accepting: Set<Int> = [200],
        submission: IdempotentSubmission? = nil,
        mapping: ErrorMapping = .statusOnly,
        timeout: TimeInterval? = nil
    ) async throws -> MonacoHTTPResponse {
        let response = try await session.send(request, route: route, timeout: timeout, submission: submission)
        if let error = Self.error(for: response, accepting: accepting, mapping: mapping) {
            throw error
        }
        return response
    }

    private struct APIErrorBody: Decodable {
        let error: String
    }

    public func getHomeDashboard() async throws -> HomeDashboardDTO {
        let url = baseURL.appending(path: "v1/home/dashboard")
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        try await applyAuthorizationHeader(to: &request)

        let response = try await send(request, route: "/v1/home/dashboard")
        return try monacoISO8601JSONDecoder().decode(HomeDashboardDTO.self, from: response.data)
    }

    public func castVote(proposalId: String, choice: String) async throws {
        let url = baseURL.appending(path: "v1/proposals/\(proposalId)/votes")
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        try await applyAuthorizationHeader(to: &request)
        request.httpBody = try JSONEncoder().encode(VoteRequestDTO(choice: choice))

        _ = try await send(request, route: "/v1/proposals/{id}/votes", accepting: [200, 204])
    }

    public func getGroupView(groupId: String) async throws -> GroupViewDTO {
        let url = baseURL.appending(path: "v1/groups/\(groupId)/view")
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        try await applyAuthorizationHeader(to: &request)

        let response = try await send(request, route: "/v1/groups/{id}/view")
        return try JSONDecoder().decode(GroupViewDTO.self, from: response.data)
    }

    public func devBuy(groupId: String, symbol: String, usdc: Int64) async throws -> DevBuyResponseDTO {
        let url = baseURL.appending(path: "v1/dev/groups/\(groupId)/buy")
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        try await applyAuthorizationHeader(to: &request)
        request.httpBody = try JSONEncoder().encode(DevBuyRequestDTO(symbol: symbol, usdc: usdc))

        // Buys the stock inside the request, so it gets the money budget, not the read one.
        let response = try await send(
            request,
            route: "/v1/dev/groups/{id}/buy",
            timeout: MonacoRequestTimeout.moneyWrite
        )
        return try JSONDecoder().decode(DevBuyResponseDTO.self, from: response.data)
    }

    private struct VoteRequestDTO: Encodable {
        let choice: String
    }

    private func applyAuthorizationHeader(to request: inout URLRequest) async throws {
        guard let accessTokenProvider,
            let token = try await accessTokenProvider(), !token.isEmpty
        else { throw MonacoAPIError.missingAccessToken }
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
    }
}
