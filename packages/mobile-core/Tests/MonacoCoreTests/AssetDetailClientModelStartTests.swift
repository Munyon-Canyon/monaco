import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailClientModelStartTests: XCTestCase {
    func testStartReadsTheAssetAndBothChartsBeforeAnyAnswerArrives() async {
        let transport = StubTransport(scripted: [.hang, .hang, .hang, .hang])
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = AssetDetailClientModel(api: api, symbol: "AAPLx", hints: FakeHintStream())

        let started = Task { await model.start() }
        await transport.waitForRequests(4)
        started.cancel()

        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.filter { $0.hasPrefix("/v1/assets/AAPLx/chart") }.count, 2)
        XCTAssertEqual(paths.filter { $0 == "/v1/assets/AAPLx" }.count, 1)
        XCTAssertEqual(paths.filter { $0 == "/v1/me/cabals" }.count, 1)
    }
}
