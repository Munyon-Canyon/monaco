import Foundation
import MonacoAPI
import MonacoRanking
import MonacoTestSupport
import XCTest

@MainActor
final class CallBoardModelTests: XCTestCase {
    private static func body(_ board: Components.Schemas.CallBoard) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(board), as: UTF8.self))
    }

    private func make(
        _ replies: [StubTransport.Reply], hints: FakeHintStream = FakeHintStream()
    ) -> (CallBoardModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (CallBoardModel(api: api, hints: hints, board: .people, period: .month, filter: .all), transport)
    }

    private func getCount(_ transport: StubTransport) async -> Int {
        await transport.sent.filter { ($0.path ?? "").hasPrefix("/v1/leaderboards/calls") }.count
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    func testLoadReadsTheMonthBoardWithOneGet() async throws {
        let (model, transport) = make([try Self.body(.peopleMonth)])
        XCTAssertEqual(model.state, .idle)

        await model.load()

        guard case .loaded(let screen) = model.state else { return XCTFail("not loaded") }
        XCTAssertEqual(screen, CallBoardScreen(.peopleMonth))
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(sent.first?.path, "/v1/leaderboards/calls?board=people&period=month&filter=all")
    }

    func testEachPeriodSwitchMakesExactlyOneGet() async throws {
        let (model, transport) = make(Array(repeating: try Self.body(.peopleMonth), count: 4))
        await model.load()

        await model.select(period: .week)
        await model.select(period: .week)
        await model.select(period: .month)
        await model.select(period: .all)

        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.count, 4, paths.description)
        XCTAssertTrue(paths[1].contains("period=week"))
        XCTAssertTrue(paths[2].contains("period=month"))
        XCTAssertTrue(paths[3].contains("period=all"))
        XCTAssertEqual(model.period, .all)
    }

    func testEachTabSwitchMakesOneGetWithItsBoardAndFilter() async throws {
        let (model, transport) = make(Array(repeating: try Self.body(.peopleMonth), count: 3))

        await model.select(tab: .cabals)
        await model.select(tab: .cabals)
        await model.select(tab: .friends)

        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.count, 2, paths.description)
        XCTAssertTrue(paths[0].contains("board=cabals") && paths[0].contains("filter=all"))
        XCTAssertTrue(paths[1].contains("board=people") && paths[1].contains("filter=friends"))
        XCTAssertEqual(model.tab, .friends)
    }

    func testAFailureThenALoadRecovers() async throws {
        let (model, _) = make([.failure(URLError(.notConnectedToInternet)), try Self.body(.peopleMonth)])

        await model.load()
        guard case .failed = model.state else { return XCTFail("expected a failure") }

        await model.load()
        guard case .loaded = model.state else { return XCTFail("expected recovery") }
    }

    func testALeaderboardsUpdatedHintWhileVisibleReloadsOnce() async throws {
        let hints = FakeHintStream()
        let (model, transport) = make(Array(repeating: try Self.body(.peopleMonth), count: 3), hints: hints)
        await model.load()
        let task = Task { await model.observe() }
        addTeardownBlock { task.cancel() }
        _ = await waitUntil { await hints.subscriberCount > 0 }

        await hints.send(.changed(.global, what: "leaderboards_updated", id: "1"))
        let arrived = await waitUntil { await self.getCount(transport) == 2 }

        XCTAssertTrue(arrived)
        for _ in 0..<50 { await Task.yield() }
        let gets = await getCount(transport)
        XCTAssertEqual(gets, 2)
    }

    func testAHintWhileHiddenWaitsForTheScreenToReturn() async throws {
        let hints = FakeHintStream()
        let (model, transport) = make(Array(repeating: try Self.body(.peopleMonth), count: 3), hints: hints)
        await model.load()
        let task = Task { await model.observe() }
        addTeardownBlock { task.cancel() }
        _ = await waitUntil { await hints.subscriberCount > 0 }
        model.setVisible(false)

        await hints.send(.changed(.global, what: "leaderboards_updated", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let hidden = await getCount(transport)
        model.setVisible(true)
        let arrived = await waitUntil { await self.getCount(transport) == 2 }

        XCTAssertEqual(hidden, 1)
        XCTAssertTrue(arrived)
    }

    #if DEBUG
    func testThePreviewModelHoldsTheStateItWasGiven() {
        let model = CallBoardModel.preview(state: .loaded(CallBoardScreen(.peopleMonth)), tab: .friends, period: .week)
        XCTAssertEqual(model.tab, .friends)
        XCTAssertEqual(model.period, .week)
        guard case .loaded = model.state else { return XCTFail("not loaded") }
    }
    #endif
}
