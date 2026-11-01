import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailAfterHoursTests: XCTestCase {
    func testKeepsTheAfterHoursChangeBesideTheSessionChange() async throws {
        var closed = Components.Schemas.AssetDetail.googl
        closed.changeBps = 659
        closed.afterHoursChangeBps = 20
        let model = try makeModel(closed)

        await model.load()

        XCTAssertEqual(model.detail?.changeBasisPoints, 659)
        XCTAssertEqual(model.detail?.afterHoursBasisPoints, 20)
        XCTAssertEqual(model.detail?.afterHoursMove?.line, "After hours +0.2%")
    }

    func testHasNoAfterHoursLineWhileTheMarketIsOpen() async throws {
        let model = try makeModel(Components.Schemas.AssetDetail.googl)

        await model.load()

        XCTAssertNil(model.detail?.afterHoursBasisPoints)
        XCTAssertNil(model.detail?.afterHoursMove)
    }

    private func makeModel(_ detail: Components.Schemas.AssetDetail) throws -> AssetDetailClientModel {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let replies: [StubTransport.Reply] = [
            .json(.ok, String(decoding: try encoder.encode(detail), as: UTF8.self)),
            .json(.ok, String(decoding: try encoder.encode(Components.Schemas.AssetChart.empty), as: UTF8.self)),
        ]
        let api = APIClient(
            serverURL: testServerURL,
            tokens: StubTokenProvider(token: "token-1"),
            transport: StubTransport(scripted: replies)
        )
        return AssetDetailClientModel(api: api, symbol: "AAPLx", hints: FakeHintStream())
    }
}
