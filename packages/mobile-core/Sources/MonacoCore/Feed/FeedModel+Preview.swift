#if DEBUG
import Foundation
import HTTPTypes
import MonacoAPI
import OpenAPIRuntime

extension FeedModel {
    public enum PreviewMode: String, Sendable {
        case items
        case empty
        case failed
    }

    public static func preview(_ mode: PreviewMode, clock: any Clock<Duration>) -> FeedModel {
        FeedModel(
            api: APIClient(
                serverURL: URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/"),
                tokens: FeedPreviewTokens(),
                transport: FeedPreviewTransport(mode: mode)),
            hints: FeedPreviewHints(),
            clock: clock)
    }
}

private struct FeedPreviewTransport: ClientTransport {
    let mode: FeedModel.PreviewMode

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        if mode == .failed { throw URLError(.notConnectedToInternet) }
        let params = URLComponents(string: request.path ?? "")?.queryItems ?? []
        let value = { (name: String) in params.first { $0.name == name }?.value }
        let kinds = value("kind").map { Set($0.split(separator: ",").map(String.init)) }
        let q = value("q")
        let items =
            mode == .empty
            ? []
            : Components.Schemas.FeedItem.samples.filter { item in
                (kinds?.contains(item.kind) ?? true)
                    && (q.map { item.title.localizedCaseInsensitiveContains($0) } ?? true)
            }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        let page = Components.Schemas.FeedPage(items: items, nextCursor: nil)
        return (response, HTTPBody(try encoder.encode(page)))
    }
}

private struct FeedPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct FeedPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
