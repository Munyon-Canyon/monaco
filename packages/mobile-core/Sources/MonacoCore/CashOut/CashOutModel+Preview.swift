#if DEBUG
import Foundation
import MonacoAPI

extension CashOutModel {
    public static func preview(_ preview: Components.Schemas.CashOutPreview) -> CashOutModel {
        CashOutModel(
            cabalID: "01890a5d-ac96-774b-bcce-b302099a8059",
            api: APIClient(
                serverURL: URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/"),
                tokens: CashOutPreviewTokens(),
                transport: CashOutPreviewTransport(preview: preview)),
            hints: CashOutPreviewHints())
    }
}

private struct CashOutPreviewTransport: ClientTransport {
    let preview: Components.Schemas.CashOutPreview

    func send(
        _: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(try encoder.encode(preview)))
    }
}

private struct CashOutPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct CashOutPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
