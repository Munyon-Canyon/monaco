#if DEBUG
import Foundation
import MonacoAPI

enum FlowScenarioAnswer: Sendable {
    case problem(status: Int, code: String, message: String)
    case dropped
}

struct FlowScenarioTransport: ClientTransport {
    let answer: FlowScenarioAnswer
    var reads: Data?

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        if request.method == .get, let reads {
            var response = HTTPResponse(status: .ok)
            response.headerFields[.contentType] = "application/json"
            return (response, HTTPBody(reads))
        }
        switch answer {
        case .dropped:
            throw URLError(.networkConnectionLost)
        case .problem(let status, let code, let message):
            var response = HTTPResponse(status: .init(code: status))
            response.headerFields[.contentType] = "application/problem+json"
            let body =
                #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","#
                + #""message":"\#(message)","trace_id":"00000000000000000000000000000000","retryable":false}"#
            return (response, HTTPBody(body))
        }
    }

    static func client(_ answer: FlowScenarioAnswer, reads: Data? = nil) -> APIClient {
        APIClient(
            serverURL: URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/"),
            tokens: FlowScenarioTokens(),
            transport: FlowScenarioTransport(answer: answer, reads: reads))
    }
}

struct FlowScenarioTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

struct FlowScenarioHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
