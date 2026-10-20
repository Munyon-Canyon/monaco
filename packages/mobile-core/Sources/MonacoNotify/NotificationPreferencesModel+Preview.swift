#if DEBUG
import Foundation
import MonacoAPI

extension NotificationPreferencesModel {
    public static func preview() -> NotificationPreferencesModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return NotificationPreferencesModel(
            api: APIClient(
                serverURL: serverURL, tokens: NotificationPreferencesPreviewTokens(),
                transport: NotificationPreferencesPreviewTransport()))
    }
}

private struct NotificationPreferencesPreviewTransport: ClientTransport {
    func send(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        if request.method == .put, let body {
            return (response, HTTPBody(try await Data(collecting: body, upTo: 1024)))
        }
        return (response, HTTPBody(#"{"decision_requests":true,"replies_and_mentions":true}"#))
    }
}

private struct NotificationPreferencesPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}
#endif
