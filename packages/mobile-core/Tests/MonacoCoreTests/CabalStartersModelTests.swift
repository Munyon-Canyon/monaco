import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CabalStartersModelTests: XCTestCase {
    func testTheFrameRowsReadInCabalsAndTheEligiblePreIpoLine() async throws {
        let transport = StubTransport(.json(.ok, try Self.encode(Components.Schemas.AssetList.inCabalsSample)))
        let model = makeModel(transport)

        await model.load()

        let starters = model.starters
        XCTAssertEqual(starters.map(\.asset.symbol), ["NVDAx", "tSpaceX", "AAPLx", "MSFTx"])
        XCTAssertEqual(starters[0].subline, "In 41 cabals")
        XCTAssertEqual(starters[0].priceMicros, 220_000_000)
        XCTAssertEqual(starters[0].changeBasisPoints, 418)
        XCTAssertNil(starters[0].eligibility)
        XCTAssertEqual(starters[1].subline, "Secondary estimate \u{00B7} in 29 cabals")
        XCTAssertEqual(starters[1].eligibility, "Eligible")
        XCTAssertNil(starters[1].changeBasisPoints)
        XCTAssertEqual(starters[2].subline, "In 29 cabals")
        XCTAssertEqual(starters[2].changeBasisPoints, -80)
    }

    func testARowWithNoCountFallsBackToTheTickerOrTheEstimateWord() {
        XCTAssertEqual(CabalStartersModel.subline(ticker: "NVDA", isPreIpo: false, cabalCount: nil), "NVDA")
        XCTAssertEqual(
            CabalStartersModel.subline(ticker: "SpaceX", isPreIpo: true, cabalCount: nil), "Secondary estimate")
    }

    func testAPreIpoRowThatCannotBeBoughtIsResearchOnly() {
        var summary = Components.Schemas.AssetSummary.starterSpaceX
        summary.tradable = false
        XCTAssertEqual(CabalStarter(summary).eligibility, "Research only")
    }

    func testItReadsTheInCabalsListOnceWithAFiveRowLimit() async throws {
        let transport = StubTransport(.json(.ok, try Self.encode(Components.Schemas.AssetList.inCabalsSample)))
        let model = makeModel(transport)

        await model.load()

        let paths: [String] = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths.count, 1)
        XCTAssertTrue(paths[0].contains("filter=in_cabals"))
        XCTAssertTrue(paths[0].contains("limit=5"))
    }

    func testAFailedReadIsFailedWithNoRows() async {
        let model = makeModel(StubTransport(.failure(URLError(.notConnectedToInternet))))

        await model.load()

        guard case .failed = model.state else { return XCTFail("got \(model.state)") }
        XCTAssertTrue(model.starters.isEmpty)
    }

    func testAPriceHintReadsTheListAgain() async throws {
        let first = try Self.encode(Components.Schemas.AssetList(assets: [.starterNvda], nextCursor: nil))
        let second = try Self.encode(Components.Schemas.AssetList.inCabalsSample)
        let hints = FakeHintStream()
        let model = makeModel(StubTransport(scripted: [.json(.ok, first), .json(.ok, second)]), hints: hints)
        await model.load()
        XCTAssertEqual(model.starters.count, 1)
        model.setVisible(true)
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        await hints.waitForSubscribers(1)

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))

        let reloaded = await waitUntil { model.starters.count == 4 }
        XCTAssertTrue(reloaded)
    }

    private func waitUntil(_ predicate: @escaping () -> Bool) async -> Bool {
        for _ in 0..<2000 {
            if predicate() { return true }
            await Task.yield()
        }
        return predicate()
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> CabalStartersModel {
        CabalStartersModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints)
    }

    private static func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }
}
