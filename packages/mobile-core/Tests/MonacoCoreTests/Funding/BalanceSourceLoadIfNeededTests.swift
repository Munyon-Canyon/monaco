import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class BalanceSourceLoadIfNeededTests: XCTestCase {
    func testJoinsAReadAlreadyUnderway() async throws {
        let transport = StubTransport(.json(.ok, BalanceSourceTests.body(available: "12000000")))
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())

        async let first: Void = model.loadIfNeeded()
        async let second: Void = model.loadIfNeeded()
        _ = await (first, second)

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/balance"])
        XCTAssertEqual(model.balance?.availableMicros, 12_000_000)
    }

    func testLeavesALoadedBalanceAlone() async throws {
        let transport = StubTransport(.json(.ok, BalanceSourceTests.body(available: "12000000")))
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())
        await model.load()

        await model.loadIfNeeded()

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/balance"])
    }

    func testRetriesAfterAFailure() async throws {
        let transport = StubTransport(.failure(URLError(.notConnectedToInternet)))
        let model = BalanceSource(api: api(transport), hints: FakeHintStream())
        await model.load()
        guard case .failed = model.state else { return XCTFail("expected a failed first read") }

        await model.loadIfNeeded()

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/balance", "/v1/me/balance"])
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }
}
