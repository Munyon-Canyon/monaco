import Foundation
import MonacoAPI
import MonacoTestSupport

enum Fixtures {
    static let ping = StubTransport.Reply.json(
        .created,
        #"{"id":"01890a5d-ac96-774b-bcce-b302099a8057","note":"hi","echoed":false}"#
    )

    static func problem(_ status: Int, _ code: String, message: String = "Server says no.") -> StubTransport.Reply {
        let body = """
            {"type":"about:blank","title":"Error","status":\(status),"code":"\(code)",\
            "message":"\(message)","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","retryable":false}
            """
        return .response(status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8))
    }

    static func client(_ transport: StubTransport, tokens: StubTokenProvider = StubTokenProvider(token: "token-1"))
        -> APIClient
    {
        APIClient(serverURL: testServerURL, tokens: tokens, transport: transport)
    }
}

extension APIClient {
    func ping(_ submission: IdempotentSubmission, note: String = "hi") async throws -> Components.Schemas.Ping {
        let body = Components.Schemas.PingRequest(note: note)
        return try await submit(submission, payload: body, operation: "postSystemPing") { client, key in
            try await client.postSystemPing(headers: .init(idempotencyKey: key), body: .json(body)).created.body.json
        }
    }

    func healthz() async throws {
        _ = try await read { try await $0.getHealthz().ok }
    }
}

extension StubTransport {
    var idempotencyKeys: [String?] {
        sent.map { $0.headerFields[.init(IdempotentSubmission.keyHeader)!] }
    }
}
