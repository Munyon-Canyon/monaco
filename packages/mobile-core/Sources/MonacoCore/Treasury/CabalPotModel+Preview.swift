#if DEBUG
import Foundation
import MonacoAPI

extension CabalPotModel {
    public static func preview(_ pot: Components.Schemas.CabalPot) -> CabalPotModel {
        CabalPotModel(
            cabalID: pot.cabalId,
            api: APIClient(
                serverURL: URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/"),
                tokens: CabalPotPreviewTokens(),
                transport: CabalPotPreviewTransport(pot: pot)),
            hints: CabalPotPreviewHints())
    }
}

private struct CabalPotPreviewTransport: ClientTransport {
    let pot: Components.Schemas.CabalPot

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
        return (response, HTTPBody(try encoder.encode(pot)))
    }
}

private struct CabalPotPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct CabalPotPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
