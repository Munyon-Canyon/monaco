import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class BalanceSourceResetTests: XCTestCase {
    func testResetForgetsTheBalanceAndTheLastErrorWithoutAToast() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, BalanceSourceTests.body(available: "17000000")),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())
        await model.load()
        await model.load()
        XCTAssertNotNil(model.lastError)

        model.reset()

        XCTAssertEqual(model.state, .idle)
        XCTAssertNil(model.balance)
        XCTAssertNil(model.lastError)
        XCTAssertEqual(model.failureTick, 1)
    }

    func testAReadStartedBeforeResetNeverLandsAfterIt() async {
        let transport = StubTransport(scripted: [.gate])
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        let reading = Task { await model.load() }
        await transport.waitForRequest()
        model.reset()
        await transport.releaseGate(.json(.ok, BalanceSourceTests.body(available: "17000000")))
        await reading.value

        XCTAssertEqual(model.state, .idle)
        XCTAssertNil(model.balance)
    }

    func testAFailureThatArrivesAfterResetIsDropped() async {
        let transport = StubTransport(scripted: [.gate])
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        let reading = Task { await model.load() }
        await transport.waitForRequest()
        model.reset()
        await transport.releaseGate(.failure(URLError(.notConnectedToInternet)))
        await reading.value

        XCTAssertEqual(model.state, .idle)
        XCTAssertNil(model.lastError)
        XCTAssertEqual(model.failureTick, 0)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }
}
