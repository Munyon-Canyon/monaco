import Foundation
import HTTPTypes
import MonacoAPI
import OpenAPIRuntime

actor ReplayingServer: ClientTransport {
    private(set) var executions = 0
    private(set) var postKeys: [String] = []
    private var answers: [String: Data] = [:]
    private var lostResponses: Int
    private let accepted: String
    private let reads: [String: String]

    init(accepted: String, reads: [String: String] = [:], lostResponses: Int = 1) {
        self.accepted = accepted
        self.reads = reads
        self.lostResponses = lostResponses
    }

    func send(
        _ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let path = request.path.flatMap { URLComponents(string: $0)?.path } ?? ""
        if path == "/v1/me/withdrawals/authorizations" {
            return Self.reply(
                .created,
                #"{"authorization_id":"01890a5d-ac96-774b-bcce-b302099a8059","expires_at":"2026-10-21T12:05:00Z"}"#)
        }
        guard request.method == .post else {
            guard let read = reads[path] else { return Self.reply(.notFound, "{}") }
            return Self.reply(.ok, read)
        }
        let key = request.headerFields[try XCTKey.name] ?? ""
        postKeys.append(key)
        if let answer = answers[key] { return Self.reply(.accepted, String(decoding: answer, as: UTF8.self)) }
        executions += 1
        answers[key] = Data(accepted.utf8)
        if lostResponses > 0 {
            lostResponses -= 1
            throw URLError(.timedOut)
        }
        return Self.reply(.accepted, accepted)
    }

    private static func reply(_ status: HTTPResponse.Status, _ body: String) -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: status)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(body))
    }
}

enum XCTKey {
    static var name: HTTPField.Name {
        get throws {
            guard let name = HTTPField.Name(IdempotentSubmission.keyHeader) else { throw URLError(.badURL) }
            return name
        }
    }
}
