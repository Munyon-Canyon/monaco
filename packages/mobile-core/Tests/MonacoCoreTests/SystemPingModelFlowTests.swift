import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class SystemPingModelFlowTests: XCTestCase {
    private let pingID = "01890a5d-ac96-774b-bcce-b302099a8057"

    func test_F00_RecordPing_ok() async {
        let transport = StubTransport(scripted: [
            .json(.created, ping(echoed: false)),
            .json(.ok, ping(echoed: true)),
        ])
        let model = makeModel(transport)

        await model.send(note: "hi")

        XCTAssertEqual(model.state, .loaded(.init(id: pingID, note: "hi", echoed: true)))
    }

    func test_F00_RecordPing_InvalidInput() async throws {
        let transport = StubTransport(scripted: [
            try .problem(problem(422, code: .invalidInput, message: "The note is too long."))
        ])
        let model = makeModel(transport)

        await model.send(note: String(repeating: "a", count: 141))

        XCTAssertEqual(model.state, .invalidInput(message: "The note is too long."))
    }

    func test_F00_RecordPing_Unauthorized() async throws {
        let transport = StubTransport(scripted: [
            try .problem(problem(401, code: .unauthorized, message: "Sign in to send a ping."))
        ])
        let model = makeModel(transport)

        await model.send(note: "hi")

        XCTAssertEqual(model.state, .unauthorized)
    }

    func test_F00_RecordPing_interrupted() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.networkConnectionLost)),
            .json(.created, ping(echoed: false)),
            .json(.ok, ping(echoed: false)),
        ])
        let model = makeModel(transport)

        await model.send(note: "hi")
        XCTAssertEqual(model.state, .failed(.transport(URLError(.networkConnectionLost))))
        await model.send(note: "hi")

        let posts = await transport.sent.filter { $0.method == .post }
        let keyHeader = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(posts.count, 2)
        XCTAssertNotNil(posts.first?.headerFields[keyHeader])
        XCTAssertEqual(Set(posts.map { $0.headerFields[keyHeader] }).count, 1)
        XCTAssertEqual(model.state, .loaded(.init(id: pingID, note: "hi", echoed: false)))
    }

    private func ping(echoed: Bool) -> String {
        #"{"id":"\#(pingID)","note":"hi","echoed":\#(echoed)}"#
    }

    private func problem(
        _ status: Int, code: Components.Schemas.ErrorCode, message: String
    ) -> Components.Schemas.Problem {
        Components.Schemas.Problem(
            _type: .about_colon_blank,
            title: "Error",
            status: status,
            code: code,
            message: message,
            traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
            retryable: false
        )
    }

    private func makeModel(_ transport: StubTransport) -> SystemPingModel {
        SystemPingModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintStream()
        )
    }
}
