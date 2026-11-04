import Foundation
import HTTPTypes
import OpenAPIRuntime

struct ChatMomentFilterMiddleware: ClientMiddleware {
    private static let maxBodyBytes = 8 * 1024 * 1024
    private static let knownKinds: Set<String> = ["cabal_created", "member_joined", "trade_filled"]

    func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let (response, responseBody) = try await next(request, body, baseURL)
        guard request.method == .get, response.status == .ok, let responseBody,
            let key = Self.listKey(for: request.path)
        else { return (response, responseBody) }
        let data = try await Data(collecting: responseBody, upTo: Self.maxBodyBytes)
        guard let filtered = Self.filtered(data, key: key) else {
            return (response, HTTPBody(data))
        }
        var headers = response.headerFields
        headers[.contentLength] = nil
        var rewritten = response
        rewritten.headerFields = headers
        return (rewritten, HTTPBody(filtered))
    }

    private static func listKey(for target: String?) -> String? {
        guard let target else { return nil }
        let path = target.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)[0]
        let parts = path.split(separator: "/", omittingEmptySubsequences: true)
        guard parts.count >= 4, parts[0] == "v1", parts[1] == "cabals", parts[3] == "messages" else { return nil }
        if parts.count == 4 { return "messages" }
        if parts.count == 6, parts[5] == "thread" { return "replies" }
        return nil
    }

    private static func filtered(_ data: Data, key: String) -> Data? {
        guard var object = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any],
            let messages = object[key] as? [[String: Any]]
        else { return nil }
        let kept = messages.filter { !hasUnknownMoment($0) }
        guard kept.count != messages.count else { return nil }
        object[key] = kept
        return try? JSONSerialization.data(withJSONObject: object)
    }

    private static func hasUnknownMoment(_ message: [String: Any]) -> Bool {
        guard message["kind"] as? String == "system", let moment = message["moment"] as? [String: Any] else {
            return false
        }
        guard let kind = moment["kind"] as? String else { return true }
        return !knownKinds.contains(kind)
    }
}
