import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailClientModelContractTests: XCTestCase {
    func testFailsWhenTheServedPriceSourceIsOutsideTheContract() async throws {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = String(decoding: try encoder.encode(Components.Schemas.AssetDetail.googl), as: UTF8.self)
        let jupiter = "\"price_source\":\"jupiter\""
        XCTAssertTrue(body.contains(jupiter))
        let seeded = body.replacingOccurrences(of: jupiter, with: "\"price_source\":\"qa-journey\"")
        let api = APIClient(
            serverURL: testServerURL,
            tokens: StubTokenProvider(token: "token-1"),
            transport: StubTransport(scripted: [.json(.ok, seeded), .json(.ok, "{}")])
        )
        let model = AssetDetailClientModel(api: api, symbol: "GOOGLx", hints: FakeHintStream())

        await model.load()

        guard case .failed = model.phase else { return XCTFail("expected failed, got \(model.phase)") }
        XCTAssertNotNil(model.lastError)
        XCTAssertEqual(model.failureTick, 1)
    }
}
