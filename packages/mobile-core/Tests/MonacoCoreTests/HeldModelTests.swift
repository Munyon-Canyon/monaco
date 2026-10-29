import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class HeldModelTests: XCTestCase {
    func testALoadIsOneHeldRead() async throws {
        let transport = StubTransport(scripted: [.json(.ok, Self.json(.sampleAndres))])
        let model = makeModel(transport)

        await model.load()

        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths, ["/v1/me/held"])
        let list = try XCTUnwrap(loaded(model))
        XCTAssertEqual(list.rows.count, 6)
        XCTAssertEqual(list.totalLine, "$19,627.71 across 3 cabals")
    }

    func testRowsKeepTheServedOrderAndNameTheirCabalAndShares() async throws {
        let model = makeModel(StubTransport(scripted: [.json(.ok, Self.json(.sampleAndres))]))

        await model.load()

        let rows = try XCTUnwrap(loaded(model)).rows
        XCTAssertEqual(rows.map(\.symbol), ["NVDAx", "AAPLx", "METAx", "tSpaceX", "AMZNx", "TSLAx"])
        XCTAssertEqual(rows[0].title, "NVIDIA")
        XCTAssertEqual(rows[0].subtitle, "AC Capital · 38 sh")
        XCTAssertEqual(rows[2].subtitle, "Degens · 4 sh")
        XCTAssertEqual(rows[5].subtitle, "Tech Titans · 2.2 sh")
        XCTAssertEqual(rows[0].valueText, "$8,360.00")
    }

    func testReturnsAreOneDecimalWithATypographicMinus() async throws {
        let model = makeModel(StubTransport(scripted: [.json(.ok, Self.json(.sampleAndres))]))

        await model.load()

        let rows = try XCTUnwrap(loaded(model)).rows
        XCTAssertEqual(rows[0].returnText, "+14.0%")
        XCTAssertEqual(rows[1].returnText, "\u{2212}1.8%")
        XCTAssertEqual(rows[4].returnText, "+4.8%")
    }

    func testAPreIpoHoldingSaysAtEstimateAndShowsNoReturn() async throws {
        let model = makeModel(StubTransport(scripted: [.json(.ok, Self.json(.sampleAndres))]))

        await model.load()

        let row = try XCTUnwrap(loaded(model)).rows[3]
        XCTAssertEqual(row.subtitle, "AC Capital · 12 sh · Pre-IPO")
        XCTAssertEqual(row.valueNote, "at estimate")
        XCTAssertNil(row.returnText)
    }

    func testAZeroCostBasisHasNoReturn() async throws {
        var list = Components.Schemas.HeldList.sampleAndres
        list.holdings[0].holding.costBasisMicros = 0
        let model = makeModel(StubTransport(scripted: [.json(.ok, Self.json(list))]))

        await model.load()

        XCTAssertNil(try XCTUnwrap(loaded(model)).rows[0].returnText)
    }

    func testOneCabalReadsSingular() async throws {
        var list = Components.Schemas.HeldList.sampleAndres
        list.cabalCount = 1
        let model = makeModel(StubTransport(scripted: [.json(.ok, Self.json(list))]))

        await model.load()

        XCTAssertEqual(try XCTUnwrap(loaded(model)).totalLine, "$19,627.71 across 1 cabal")
    }

    func testNoHoldingsLoadsAnEmptyList() async throws {
        let model = makeModel(StubTransport(scripted: [.json(.ok, Self.json(.sampleEmpty))]))

        await model.load()

        let list = try XCTUnwrap(loaded(model))
        XCTAssertTrue(list.rows.isEmpty)
    }

    func testAFailedFirstLoadFailsAndALaterLoadRecovers() async throws {
        let transport = StubTransport(scripted: [
            .json(.internalServerError, "{}"), .json(.ok, Self.json(.sampleAndres)),
        ])
        let model = makeModel(transport)

        await model.load()
        guard case .failed = model.state else { return XCTFail("\(model.state)") }
        await model.load()

        XCTAssertEqual(try XCTUnwrap(loaded(model)).rows.count, 6)
    }

    func testAnActivityHintFromAHeldCabalReloads() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.json(.sampleAndres)), .json(.ok, Self.json(.sampleEmpty)),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.load()
        let observing = Task { await model.observe() }
        await hints.waitForSubscribers(5)

        await hints.send(.changed(.cabal("01890a5d-ac96-774b-bcce-b302099a8059"), what: "activity_changed", id: "1"))
        let reloaded = await waitUntil { self.loaded(model)?.rows.isEmpty == true }

        observing.cancel()
        XCTAssertTrue(reloaded)
    }

    func testAPriceHintReloads() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.json(.sampleAndres)), .json(.ok, Self.json(.sampleEmpty)),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.load()
        let observing = Task { await model.observe() }
        await hints.waitForSubscribers(5)

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))
        let reloaded = await waitUntil { self.loaded(model)?.rows.isEmpty == true }

        observing.cancel()
        XCTAssertTrue(reloaded)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<2000 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private func loaded(_ model: HeldModel) -> HeldList? {
        if case .loaded(let list) = model.state { return list }
        return nil
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> HeldModel {
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return HeldModel(api: api, hints: hints, clock: ContinuousClock())
    }

    private static func json(_ list: Components.Schemas.HeldList) -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: (try? encoder.encode(list)) ?? Data(), as: UTF8.self)
    }
}
