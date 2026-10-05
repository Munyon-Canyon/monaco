import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalPotModelTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"
    private var path: String { "/v1/cabals/\(cabalID)/pot" }

    func testLoadSendsOneGetAndSummarisesThePot() async throws {
        let transport = StubTransport(scripted: [.json(.ok, Self.pot(valueMicros: 1_000_000_000))])
        let model = CabalPotModel(cabalID: cabalID, api: api(transport), hints: FakeHintStream())

        await model.load()

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, [path])
        XCTAssertEqual(model.summary?.potValue, "$1,000.00")
        XCTAssertEqual(model.summary?.slice, CabalPotSummary.Slice.none)
    }

    func testActivityChangedOnThisCabalRefetchesThePot() async throws {
        try await assertRefetch(after: .changed(.cabal(cabalID), what: "activity_changed", id: "1"))
    }

    func testPricesUpdatedRefetchesThePot() async throws {
        try await assertRefetch(after: .changed(.global, what: "prices_updated", id: "1"))
    }

    func testAResyncRefetchesThePot() async throws {
        try await assertRefetch(after: .resync)
    }

    func testHintsForAnotherCabalOrAnotherTopicSendNothing() async throws {
        let transport = StubTransport(.json(.ok, Self.pot(valueMicros: 1_000_000_000)))
        let hints = FakeHintStream()
        let model = CabalPotModel(cabalID: cabalID, api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 2 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.cabal("01890a5d-ac96-774b-bcce-b302099a8061"), what: "activity_changed", id: "1"))
        await hints.send(.changed(.cabal(cabalID), what: "members", id: "2"))
        await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "3"))
        let marker = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(marker)
        for _ in 0..<50 { await Task.yield() }

        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testAFailedReloadKeepsTheSummaryAndSetsAToast() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.pot(valueMicros: 1_000_000_000)),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = CabalPotModel(cabalID: cabalID, api: api(transport), hints: FakeHintStream())

        await model.load()
        await model.load()

        XCTAssertEqual(model.summary?.potValue, "$1,000.00")
        XCTAssertEqual(model.toast, "You're offline. Try again.")
        model.dismissToast()
        XCTAssertNil(model.toast)
    }

    func testAFailedFirstLoadFailsWithoutAToast() async throws {
        let transport = StubTransport(scripted: [.failure(URLError(.notConnectedToInternet))])
        let model = CabalPotModel(cabalID: cabalID, api: api(transport), hints: FakeHintStream())

        await model.load()

        guard case .failed = model.state else {
            XCTFail("expected a failure, got \(model.state)")
            return
        }
        XCTAssertNil(model.toast)
    }

    func testThePreviewServesItsFixtureThroughTheClient() async {
        for (pot, state) in [
            (Components.Schemas.CabalPot.sampleInvested, CabalPotSummary.PotState.invested),
            (.sampleCashOnly, .cashOnly), (.sampleZero, .zero), (.sampleOutsider, .invested),
        ] {
            let model = CabalPotModel.preview(pot)

            await model.load()

            XCTAssertEqual(model.summary, CabalPotSummary(pot))
            XCTAssertEqual(model.summary?.state, state)
        }
    }

    private func assertRefetch(after hint: Hint, file: StaticString = #filePath, line: UInt = #line) async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.pot(valueMicros: 1_000_000_000)),
            .json(.ok, Self.pot(valueMicros: 2_000_000_000)),
        ])
        let hints = FakeHintStream()
        let model = CabalPotModel(cabalID: cabalID, api: api(transport), hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 2 }
        XCTAssertTrue(subscribed, file: file, line: line)

        await hints.send(hint)
        let refetched = await waitUntil { model.summary?.potValue == "$2,000.00" }

        XCTAssertTrue(refetched, file: file, line: line)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, [path, path], file: file, line: line)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    static func pot(valueMicros: Int64) -> String {
        """
        {"cabal_id":"01890a5d-ac96-774b-bcce-b302099a8060","pot_value_micros":\(valueMicros),\
        "cash_micros":\(valueMicros),"cash_weight_bps":10000,"pnl_micros":0,"return_bps":null,\
        "prices_as_of":"2026-10-03T15:00:00Z","holdings":[],\
        "me":{"share_units":0,"value_micros":0,"slice_bps":0,"net_contributed_micros":0,"pnl_micros":0}}
        """
    }
}
