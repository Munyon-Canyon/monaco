import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CabalsTopModelTests: XCTestCase {
    private static let path = "/v1/leaderboards/calls?board=cabals&period=month"

    private static func rows() -> [Components.Schemas.CallBoardRow] {
        let board = Components.Schemas.CallBoard.cabalsMonth
        var top = board.top
        top[1].isMine = false
        top[2].isMine = false
        var nightShift = top[2]
        nightShift.subject.name = "Night Shift"
        nightShift.ratePct = 75
        nightShift.right = 6
        nightShift.resolved = 8
        nightShift.isMine = false
        return [top[0], top[1], nightShift]
    }

    private static func board(_ rows: [Components.Schemas.CallBoardRow]) -> Components.Schemas.CallBoard {
        var board = Components.Schemas.CallBoard.cabalsMonth
        board.top = rows
        return board
    }

    private static func reply(_ board: Components.Schemas.CallBoard) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(board), as: UTF8.self))
    }

    private func make(
        _ replies: [StubTransport.Reply], hints: FakeHintStream = FakeHintStream()
    ) -> (CabalsTopModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return (CabalsTopModel(api: api, hints: hints), transport)
    }

    private func getCount(_ transport: StubTransport) async -> Int {
        await transport.sent.filter { $0.path == Self.path }.count
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    func testTheTopThreeShowTheirLinesAndTheModelReadsTheMonthsCabalsBoardOnce() async throws {
        let (model, transport) = make([try Self.reply(Self.board(Self.rows()))])
        XCTAssertEqual(model.phase, .loading)

        await model.load()

        guard case .ready(let tiles) = model.phase else { return XCTFail("not ready: \(model.phase)") }
        XCTAssertEqual(tiles.map(\.name), ["Pilates", "Tech Titans", "Night Shift"])
        XCTAssertEqual(
            tiles.map(\.line), ["93% right · 13 of 14", "89% right · 8 of 9", "75% right · 6 of 8"])
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(sent.first?.path, Self.path)
    }

    func testTheLineIsPercentThenRightOfResolved() async throws {
        XCTAssertEqual(CabalsTopModel.line(ratePct: 93, right: 13, resolved: 14), "93% right · 13 of 14")
    }

    func testARowTheViewerBelongsToOrWithNoRateIsLeftOut() async throws {
        var rows = Self.rows()
        rows[0].isMine = true
        rows[1].ratePct = nil
        let tiles = CabalsTopModel.tiles(Self.board(rows))
        XCTAssertEqual(tiles.map(\.name), ["Night Shift"])
    }

    func testTilesKeepTheBoardOrderPictureAndId() async throws {
        var rows = Self.rows()
        rows[0].subject.pictureUrl = "https://example.test/p.png"
        let tiles = CabalsTopModel.tiles(Self.board(rows))
        XCTAssertEqual(tiles.first?.pictureURL, "https://example.test/p.png")
        XCTAssertEqual(tiles.map(\.id), rows.map(\.subject.id))
    }

    func testMoreThanThreeEligibleRowsShowThree() async throws {
        let rows = Self.rows()
        let tiles = CabalsTopModel.tiles(Self.board(rows + [rows[0]]))
        XCTAssertEqual(tiles.count, 3)
    }

    func testWhenEveryTopRowIsLeftOutTheBlockIsHidden() async throws {
        var rows = Self.rows()
        rows[0].isMine = true
        rows[1].ratePct = nil
        rows[2].isMine = true
        let (model, _) = make([try Self.reply(Self.board(rows))])
        await model.load()
        XCTAssertEqual(model.phase, .hidden)
    }

    func testAFailedFirstReadHidesTheBlock() async throws {
        let (model, _) = make([.json(.internalServerError, "{}")])
        await model.load()
        XCTAssertEqual(model.phase, .hidden)
    }

    func testAFailedRefreshKeepsTheTiles() async throws {
        let (model, _) = make([try Self.reply(Self.board(Self.rows())), .json(.internalServerError, "{}")])
        await model.load()
        await model.load()
        guard case .ready(let tiles) = model.phase else { return XCTFail("lost the tiles: \(model.phase)") }
        XCTAssertEqual(tiles.count, 3)
    }

    func testACancelledFirstLoadChangesNothing() async throws {
        let (model, transport) = make([.hang])
        let load = Task { await model.load() }
        let started = await waitUntil { await transport.sent.count >= 1 }
        XCTAssertTrue(started)
        load.cancel()
        await load.value
        XCTAssertEqual(model.phase, .loading)
    }

    func testACancelledLoadThatStillGetsABoardChangesNothing() async throws {
        let (model, transport) = make([.gate])
        let load = Task { await model.load() }
        let started = await waitUntil { await transport.sent.count >= 1 }
        XCTAssertTrue(started)
        load.cancel()
        await transport.releaseGate(try Self.reply(Self.board(Self.rows())))
        await load.value
        XCTAssertEqual(model.phase, .loading)
    }

    func testALeaderboardsUpdatedHintReloadsOnlyWhileVisible() async throws {
        let hints = FakeHintStream()
        let reply = try Self.reply(Self.board(Self.rows()))
        let (model, transport) = make([reply, reply, reply], hints: hints)
        await model.load()
        let task = Task { await model.observe() }
        addTeardownBlock { task.cancel() }
        _ = await waitUntil { await hints.subscriberCount > 0 }
        model.setVisible(false)

        await hints.send(.changed(.global, what: "leaderboards_updated", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await getCount(transport)
        model.setVisible(true)
        let arrived = await waitUntil { await self.getCount(transport) == 2 }

        XCTAssertEqual(whileHidden, 1)
        XCTAssertTrue(arrived)
    }

    func testAHintWhileVisibleReloadsOnce() async throws {
        let hints = FakeHintStream()
        let reply = try Self.reply(Self.board(Self.rows()))
        let (model, transport) = make([reply, reply, reply], hints: hints)
        await model.load()
        let task = Task { await model.observe() }
        addTeardownBlock { task.cancel() }
        _ = await waitUntil { await hints.subscriberCount > 0 }

        await hints.send(.changed(.global, what: "leaderboards_updated", id: "1"))
        let arrived = await waitUntil { await self.getCount(transport) == 2 }
        for _ in 0..<50 { await Task.yield() }
        let reads = await getCount(transport)

        XCTAssertTrue(arrived)
        XCTAssertEqual(reads, 2)
    }
}
