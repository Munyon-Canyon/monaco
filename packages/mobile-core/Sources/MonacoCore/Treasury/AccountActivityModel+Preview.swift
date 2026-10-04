#if DEBUG
import Foundation
import MonacoAPI

extension AccountActivityModel {
    public static func preview(empty: Bool, clock: @escaping @Sendable () -> Date) -> AccountActivityModel {
        let pages: [String?: Components.Schemas.UserTxnPage] =
            empty
            ? [nil: .sampleEmpty]
            : [nil: .sampleFirst, Components.Schemas.UserTxnPage.sampleCursor: .sampleSecond]
        return AccountActivityModel(
            api: APIClient(
                serverURL: URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/"),
                tokens: AccountActivityPreviewTokens(),
                transport: AccountActivityPreviewTransport(pages: pages)),
            hints: AccountActivityPreviewHints(),
            clock: clock)
    }
}

private struct AccountActivityPreviewTransport: ClientTransport {
    let pages: [String?: Components.Schemas.UserTxnPage]

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let cursor = URLComponents(string: request.path ?? "")?.queryItems?.first { $0.name == "cursor" }?.value
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(try encoder.encode(pages[cursor] ?? .sampleEmpty)))
    }
}

private struct AccountActivityPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct AccountActivityPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
