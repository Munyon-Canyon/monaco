import Foundation
import HTTPTypes
import MonacoAPI
import OpenAPIRuntime

actor StubTransport: ClientTransport {
    private let status: HTTPResponse.Status
    private let contentType: String
    private let body: Data
    private(set) var sent: [HTTPRequest] = []

    init(status: HTTPResponse.Status, contentType: String, body: Data) {
        self.status = status
        self.contentType = contentType
        self.body = body
    }

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        sent.append(request)
        var response = HTTPResponse(status: status)
        response.headerFields[.contentType] = contentType
        return (response, HTTPBody(body))
    }

    static func ok(_ text: String) -> StubTransport {
        StubTransport(status: .ok, contentType: "text/plain", body: Data(text.utf8))
    }

    static func problem(_ problem: Components.Schemas.Problem) throws -> StubTransport {
        StubTransport(
            status: .init(code: problem.status),
            contentType: "application/problem+json; charset=utf-8",
            body: try JSONEncoder().encode(problem)
        )
    }
}

let testServerURL = URL(string: "http://api.test")!
