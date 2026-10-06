import Foundation
import MonacoAPI
import MonacoCore

enum MonacoAPIError: Error {
    case invalidResponse
    case httpStatus(Int)
    case apiError(status: Int, message: String)
    case missingAccessToken
}

extension Error {
    /// SwiftUI `.task` cancellation usually surfaces as `URLError.cancelled`, not `CancellationError`.
    var isRequestCancellation: Bool {
        if self is CancellationError {
            return true
        }
        if let urlError = self as? URLError {
            return urlError.code == .cancelled
        }
        let nsError = self as NSError
        return nsError.domain == NSURLErrorDomain && nsError.code == NSURLErrorCancelled
    }
}

final class MonacoAPIClient: AppSessionDataSource {
    private let baseURL: URL
    /// Every request goes through the transport so an expired access token is
    /// refreshed and the request retried once instead of signing the user out.
    private let session: MonacoHTTPTransport

    init(baseURL: URL = Config.apiBaseURL, session: URLSession = .monaco) {
        self.baseURL = baseURL
        self.session = MonacoHTTPTransport(session: session)
    }

    func health() async throws -> HealthResponse {
        let url = baseURL.appending(path: "health")
        let (data, response) = try await session.data(from: url)
        guard let http = response as? HTTPURLResponse else {
            throw MonacoAPIError.invalidResponse
        }
        guard http.statusCode == 200 else {
            throw MonacoAPIError.httpStatus(http.statusCode)
        }
        return try JSONDecoder().decode(HealthResponse.self, from: data)
    }

    func getHomeDashboard(accessToken: String) async throws -> HomeDashboardDTO {
        let url = baseURL.appending(path: "v1/home/dashboard")
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        try applyAuthorizationHeader(accessToken: accessToken, to: &request)

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw MonacoAPIError.invalidResponse
        }
        guard http.statusCode == 200 else {
            throw MonacoAPIError.httpStatus(http.statusCode)
        }
        return try monacoISO8601JSONDecoder().decode(HomeDashboardDTO.self, from: data)
    }

    func getGroup(accessToken: String, groupId: String) async throws -> GetGroupResponse {
        let url = baseURL.appending(path: "v1/groups/\(groupId)")
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        try applyAuthorizationHeader(accessToken: accessToken, to: &request)

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw MonacoAPIError.invalidResponse
        }
        guard http.statusCode == 200 else {
            throw MonacoAPIError.httpStatus(http.statusCode)
        }
        return try JSONDecoder().decode(GetGroupResponse.self, from: data)
    }

    func getGroupView(accessToken: String, groupId: String) async throws -> GroupViewDTO {
        let url = baseURL.appending(path: "v1/groups/\(groupId)/view")
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        try applyAuthorizationHeader(accessToken: accessToken, to: &request)

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw MonacoAPIError.invalidResponse
        }
        guard http.statusCode == 200 else {
            throw MonacoAPIError.httpStatus(http.statusCode)
        }
        return try JSONDecoder().decode(GroupViewDTO.self, from: data)
    }

    /// What the caller's own cabals are doing with one stock: holdings, open votes
    /// and activity. Scoped server-side to the caller's memberships.
    func getAssetSocial(
        accessToken: String,
        symbol: String
    ) async throws -> AssetSocialDTO {
        let url = baseURL.appending(path: "v1/assets/\(symbol)/social")
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        try applyAuthorizationHeader(accessToken: accessToken, to: &request)

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw MonacoAPIError.invalidResponse
        }
        guard http.statusCode == 200 else {
            throw MonacoAPIError.httpStatus(http.statusCode)
        }
        return try JSONDecoder().decode(AssetSocialDTO.self, from: data)
    }

    func devBuy(accessToken: String, groupId: String, symbol: String, usdc: Int64) async throws -> DevBuyResponse {
        let url = baseURL.appending(path: "v1/dev/groups/\(groupId)/buy")
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        try applyAuthorizationHeader(accessToken: accessToken, to: &request)
        request.httpBody = try JSONEncoder().encode(DevBuyRequest(symbol: symbol, usdc: usdc))

        // Buys the stock inside the request, so it gets the money budget, not the read one.
        let (data, response) = try await session.data(for: request, timeout: MonacoRequestTimeout.moneyWrite)
        guard let http = response as? HTTPURLResponse else {
            throw MonacoAPIError.invalidResponse
        }
        guard http.statusCode == 200 else {
            throw MonacoAPIError.httpStatus(http.statusCode)
        }
        return try JSONDecoder().decode(DevBuyResponse.self, from: data)
    }

}

extension MonacoAPIClient {
    private func applyAuthorizationHeader(accessToken: String, to request: inout URLRequest) throws {
        let token = accessToken.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !token.isEmpty else {
            throw MonacoAPIError.missingAccessToken
        }
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
    }
}
