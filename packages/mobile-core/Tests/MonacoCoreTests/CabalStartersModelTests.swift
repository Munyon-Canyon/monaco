import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalStartersModelTests: XCTestCase {
    func testTheRowsCarryTheCabalCountAndOneReadFillsThem() async throws {
        let (model, transport) = try make(.json(.ok, Self.list(.inCabalsSample)))

        await model.load()

        let nvidia = try XCTUnwrap(model.rows.first)
        XCTAssertEqual(nvidia.subline, "In 41 cabals")
        XCTAssertFalse(nvidia.isEligible)
        XCTAssertEqual(model.rows.count, 4)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths, ["/v1/assets?filter=in_cabals&limit=5"])
    }

    func testAPreIpoRowReadsSecondaryEstimateAndEligible() async throws {
        let (model, _) = try make(.json(.ok, Self.list(.inCabalsSample)))

        await model.load()

        let spaceX = try XCTUnwrap(model.rows.first { $0.asset.symbol == "tSpaceX" })
        XCTAssertEqual(spaceX.subline, "Secondary estimate \u{00B7} in 29 cabals")
        XCTAssertTrue(spaceX.isEligible)
        XCTAssertEqual(CabalStartersModel.eligible, "Eligible")
    }

    func testARowWithNoCountFallsBackToTheTickerOrTheEstimate() {
        XCTAssertEqual(CabalStarter.subline(kind: .stock, ticker: "AAPL", cabalCount: nil), "AAPL")
        XCTAssertEqual(CabalStarter.subline(kind: .preIpo, ticker: "SpaceX", cabalCount: nil), "Secondary estimate")
    }

    func testAFailedReadKeepsTheRows() async throws {
        let (model, _) = try make(
            .json(.ok, Self.list(.inCabalsSample)), .failure(URLError(.notConnectedToInternet)))

        await model.load()
        await model.load()

        XCTAssertEqual(model.rows.count, 4)
    }

    func testAPriceHintReloads() async throws {
        let (model, transport, hints) = try makeObserved(
            .json(.ok, Self.list(.inCabalsSample)),
            .json(.ok, Self.list(.init(assets: [.starterNvda], nextCursor: nil))))
        await model.load()
        model.setVisible(true)
        let observer = Task { await model.observe() }
        await hints.waitForSubscribers(1)

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))

        for _ in 0..<500 {
            if model.rows.count == 1 { break }
            await Task.yield()
        }
        XCTAssertEqual(model.rows.count, 1)
        let reads = await transport.sent.count
        XCTAssertEqual(reads, 2)
        observer.cancel()
    }

    private func make(_ replies: StubTransport.Reply...) throws -> (CabalStartersModel, StubTransport) {
        let (model, transport, _) = try makeObserved(replies)
        return (model, transport)
    }

    private func makeObserved(_ replies: StubTransport.Reply...) throws -> (
        CabalStartersModel, StubTransport, FakeHintStream
    ) {
        try makeObserved(replies)
    }

    private func makeObserved(_ replies: [StubTransport.Reply]) throws -> (
        CabalStartersModel, StubTransport, FakeHintStream
    ) {
        let transport = StubTransport(routes: ["/v1/assets?filter=in_cabals&limit=5": replies])
        let hints = FakeHintStream()
        let model = CabalStartersModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints)
        return (model, transport, hints)
    }

    private static func list(_ list: Components.Schemas.AssetList) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(list), as: UTF8.self)
    }
}
