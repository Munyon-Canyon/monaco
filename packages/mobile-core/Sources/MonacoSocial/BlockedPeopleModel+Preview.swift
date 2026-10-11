#if DEBUG
import Foundation
import MonacoAPI
import MonacoFlows

extension BlockedPeopleModel {
    public static func preview(answering scenario: Flow20aScenario? = nil, empty: Bool = false) -> BlockedPeopleModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return BlockedPeopleModel(
            api: APIClient(
                serverURL: serverURL, tokens: BlockedPeoplePreviewTokens(),
                transport: BlockedPeoplePreviewTransport(refusal: scenario, empty: empty)),
            hints: BlockedPeoplePreviewHints()
        )
    }
}

private struct BlockedPeoplePreviewTransport: ClientTransport {
    let refusal: Flow20aScenario?
    let empty: Bool

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        guard request.method == .delete else {
            var response = HTTPResponse(status: .ok)
            response.headerFields[.contentType] = "application/json"
            let users = empty ? Components.Schemas.BlockedUsers.sampleEmpty : Components.Schemas.BlockedUsers.sample
            return (response, HTTPBody(try JSONEncoder().encode(users)))
        }
        switch refusal {
        case nil:
            return (HTTPResponse(status: .noContent), nil)
        case .cannotBlockSelf:
            return Self.problem(400, code: "cannot_block_self", message: "You can't block yourself.")
        case .userNotFound:
            return Self.problem(404, code: "user_not_found", message: "That account isn't available.")
        case .reportTargetNotFound:
            return Self.problem(404, code: "report_target_not_found", message: "That can't be reported anymore.")
        case .rateLimited:
            return Self.problem(429, code: "rate_limited", message: "Too many tries. Wait a moment.")
        case .unauthorized:
            return Self.problem(401, code: "unauthorized", message: "Sign in to continue.")
        case .interrupted:
            throw URLError(.networkConnectionLost)
        }
    }

    private static func problem(_ status: Int, code: String, message: String) -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .init(code: status))
        response.headerFields[.contentType] = "application/problem+json"
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","#
            + #""message":"\#(message)","trace_id":"00000000000000000000000000000000","retryable":false}"#
        return (response, HTTPBody(body))
    }
}

private struct BlockedPeoplePreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct BlockedPeoplePreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { continuation in
            continuation.finish()
        }
    }
}
#endif
