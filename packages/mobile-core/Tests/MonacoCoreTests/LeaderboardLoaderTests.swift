import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class LeaderboardLoaderTests: XCTestCase {
    private typealias Page = Components.Schemas.LeaderboardPage
    private typealias Row = Components.Schemas.LeaderboardRow

    func testLoadReadsTheFirstPageOfTheBoard() async throws {
        let (loader, transport, _) = try make(.people, [.page(.samplePeople())])

        await loader.load()

        XCTAssertEqual(loader.rows.map(\.rank), [1, 2, 3])
        XCTAssertEqual(loader.rows.first?.name, "Investor 1")
        XCTAssertEqual(loader.phase, .loaded)
        XCTAssertEqual(loader.computedAt, Page.sampleComputedAt)
        XCTAssertEqual(loader.freshness(now: Page.sampleComputedAt.addingTimeInterval(600)), "Updated 10 min ago")
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/leaderboards/people?range=ALL&filter=all"])
    }

    func testEachBoardReadsItsOwnRoute() async throws {
        let (cabals, cabalsTransport, _) = try make(.cabals, [.page(.sampleEmpty)])
        let (members, membersTransport, _) = try make(.cabalMembers(id: "c1"), [.page(.sampleEmpty)])

        await cabals.load()
        await members.load()

        let cabalsPaths = await cabalsTransport.sent.map(\.path)
        let membersPaths = await membersTransport.sent.map(\.path)
        XCTAssertEqual(cabalsPaths, ["/v1/leaderboards/cabals?range=ALL"])
        XCTAssertEqual(membersPaths, ["/v1/cabals/c1/leaderboard?range=ALL"])
    }

    func testAnEmptyBoardIsEmpty() async throws {
        let (loader, _, _) = try make(.people, [.page(.sampleEmpty)])

        await loader.load()

        XCTAssertEqual(loader.phase, .empty)
    }

    func testAFailedFirstLoadIsTheErrorStateWithNoToast() async throws {
        let (loader, _, _) = try make(.people, [.failure])

        await loader.load()

        guard case .failed(.transport) = loader.phase else { return XCTFail("want a transport failure") }
        XCTAssertNil(loader.toast)
    }

    func testMeIsPinnedOnlyWhenItsRowIsNotOnThePage() async throws {
        let me = Row.sample(rank: 42, id: "me-1", name: "Me")
        let (loader, _, _) = try make(.people, [.page(.samplePeople(me: me))])

        await loader.load()

        XCTAssertEqual(loader.pinnedMe?.id, "me-1")
        XCTAssertEqual(loader.pinnedMe?.rank, 42)
        XCTAssertEqual(loader.pinnedMe?.isViewer, true)
        XCTAssertFalse(loader.rows.contains(where: \.isViewer))
    }

    func testTheViewersRowOnThePageIsMarkedFromMe() async throws {
        let me = Row.sample(rank: 2, id: "user-2", name: "Investor 2")
        let (loader, _, _) = try make(.people, [.page(.samplePeople(me: me))])

        await loader.load()

        XCTAssertNil(loader.pinnedMe)
        XCTAssertEqual(loader.rows.filter(\.isViewer).map(\.id), ["user-2"])
    }

    func testARowCarriesItsFiguresAndTheDelayedPricesFlag() async throws {
        let rows = [
            Row.sample(rank: 1, id: "a", name: "A", returnBps: nil, flags: [.stalePrices]),
            Row.sample(rank: 2, id: "b", name: "B", valueMicros: 5, pnlMicros: -3, returnBps: -40),
        ]
        let (loader, _, _) = try make(.people, [.page(Page.samplePeople(count: 0).with(rows: rows))])

        await loader.load()

        XCTAssertEqual(loader.rows[0].returnBps, nil)
        XCTAssertTrue(loader.rows[0].pricesDelayed)
        XCTAssertEqual(loader.rows[1].valueMicros, 5)
        XCTAssertEqual(loader.rows[1].pnlMicros, -3)
        XCTAssertEqual(loader.rows[1].returnBps, -40)
        XCTAssertFalse(loader.rows[1].pricesDelayed)
    }

    func testLoadMoreAppendsTheNextPageByCursor() async throws {
        let (loader, transport, _) = try make(
            .people, [.page(.samplePeople(count: 2, nextCursor: "Mg")), .page(.samplePeople(firstRank: 3, count: 2))])
        await loader.load()
        XCTAssertTrue(loader.hasMore)

        await loader.loadMore()

        XCTAssertEqual(loader.rows.map(\.rank), [1, 2, 3, 4])
        XCTAssertFalse(loader.hasMore)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/leaderboards/people?range=ALL&cursor=Mg&filter=all")
    }

    func testAHintReadsTheFirstPageOnceAndRecordsTheMoves() async throws {
        let first = Page.samplePeople(count: 3)
        let order = [first.rows[1], first.rows[0], first.rows[2]]
        let swapped = first.with(rows: order.enumerated().map { $1.ranked($0 + 1) })
        let (loader, transport, hints) = try make(.people, [.page(first), .page(swapped)])
        await loader.load()
        let task = Task { await loader.observe() }
        while await hints.subscriberCount < 1 { await Task.yield() }

        await hints.send(.changed(.global, what: "leaderboards_updated", id: "1"))
        await transport.waitForRequests(2)
        while loader.moves.isEmpty { await Task.yield() }

        XCTAssertEqual(loader.rows.map(\.id), ["user-2", "user-1", "user-3"])
        XCTAssertEqual(loader.moves, ["user-2": 1, "user-1": -1])
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
        task.cancel()
    }

    func testAnotherHintIsIgnored() async throws {
        let (loader, transport, hints) = try make(.people, [.page(.samplePeople())])
        await loader.load()
        let task = Task { await loader.observe() }
        while await hints.subscriberCount < 1 { await Task.yield() }

        await hints.send(.changed(.global, what: "feed", id: "1"))
        for _ in 0..<50 { await Task.yield() }

        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
        task.cancel()
    }

    func testResyncReadsTheFirstPageAgain() async throws {
        let (loader, transport, hints) = try make(.people, [.page(.samplePeople()), .page(.samplePeople(count: 2))])
        await loader.load()
        let task = Task { await loader.observe() }
        while await hints.subscriberCount < 1 { await Task.yield() }

        await hints.send(.resync)
        await transport.waitForRequests(2)
        while loader.rows.count != 2 { await Task.yield() }

        XCTAssertEqual(loader.rows.count, 2)
        task.cancel()
    }

    func testRapidRangeTapsSendOneRequestForTheFinalRange() async throws {
        let (loader, transport, _) = try make(.people, [.page(.samplePeople()), .page(.samplePeople(range: ._1w))])
        await loader.load()

        loader.select(range: .oneHour)
        loader.select(range: .oneDay)
        loader.select(range: .oneWeek)
        await transport.waitForRequests(2)
        while loader.isLoading { await Task.yield() }

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(
            paths, ["/v1/leaderboards/people?range=ALL&filter=all", "/v1/leaderboards/people?range=1W&filter=all"])
        XCTAssertEqual(loader.range, .oneWeek)
    }

    func testFriendsReadsTheFriendsFilter() async throws {
        let (loader, transport, _) = try make(.people, [.page(.samplePeople()), .page(.sampleEmpty)])
        await loader.load()

        loader.select(filter: .friends)
        await transport.waitForRequests(2)
        while loader.isLoading { await Task.yield() }

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/leaderboards/people?range=ALL&filter=friends")
        XCTAssertEqual(loader.phase, .empty)
    }

    func testFriendsWithOnlyTheViewersRowIsEmpty() async throws {
        let me = Row.sample(rank: 1, id: "me-1", name: "Me")
        let alone = Page.samplePeople(count: 0, me: me).with(rows: [me])
        let (loader, transport, _) = try make(.people, [.page(.samplePeople()), .page(alone)])
        await loader.load()
        XCTAssertEqual(loader.phase, .loaded)

        loader.select(filter: .friends)
        await transport.waitForRequests(2)
        while loader.isLoading { await Task.yield() }

        XCTAssertEqual(loader.phase, .empty)
    }

    func testFriendsWithAFollowedPersonIsLoaded() async throws {
        let me = Row.sample(rank: 2, id: "me-1", name: "Me")
        let friend = Row.sample(rank: 1, id: "friend-1", name: "Friend")
        let page = Page.samplePeople(count: 0, me: me).with(rows: [friend, me])
        let (loader, _, _) = try make(.people, [.page(page)])
        loader.select(filter: .friends)
        while loader.phase == .loading { await Task.yield() }

        XCTAssertEqual(loader.phase, .loaded)
        XCTAssertEqual(loader.rows.map(\.id), ["friend-1", "me-1"])
    }

    func testAFailedRangeChangeKeepsTheRowsAndToasts() async throws {
        let (loader, _, _) = try make(.people, [.page(.samplePeople()), .failure])
        await loader.load()

        loader.select(range: .oneDay)
        while loader.toast == nil { await Task.yield() }

        XCTAssertEqual(loader.rows.count, 3)
        XCTAssertEqual(loader.phase, .loaded)
        XCTAssertEqual(loader.toast, "You're offline. Try again.")
        loader.dismissToast()
        XCTAssertNil(loader.toast)
    }

    func testTappingTheSelectedRangeAgainReadsOnlyAfterAFailure() async throws {
        let (loader, transport, _) = try make(
            .people, [.page(.samplePeople()), .failure, .page(.samplePeople(range: ._1d))])
        await loader.load()
        loader.select(range: .all)
        for _ in 0..<50 { await Task.yield() }
        var count = await transport.sent.count
        XCTAssertEqual(count, 1)

        loader.select(range: .oneDay)
        while loader.toast == nil || loader.isLoading { await Task.yield() }
        loader.select(range: .oneDay)
        await transport.waitForRequests(3)
        while loader.isLoading { await Task.yield() }

        count = await transport.sent.count
        XCTAssertEqual(count, 3)
    }

    private enum Script {
        case page(Page)
        case failure

        func reply() throws -> StubTransport.Reply {
            switch self {
            case .page(let page):
                let encoder = JSONEncoder()
                encoder.dateEncodingStrategy = .iso8601
                return .json(.ok, String(decoding: try encoder.encode(page), as: UTF8.self))
            case .failure:
                return .failure(URLError(.notConnectedToInternet))
            }
        }
    }

    private func make(
        _ board: LeaderboardBoard, _ script: [Script]
    ) throws -> (LeaderboardLoader, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let hints = FakeHintStream()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (LeaderboardLoader(board: board, api: api, hints: hints), transport, hints)
    }
}

extension Components.Schemas.LeaderboardPage {
    fileprivate func with(rows: [Components.Schemas.LeaderboardRow]) -> Self {
        var page = self
        page.rows = rows
        return page
    }
}

extension Components.Schemas.LeaderboardRow {
    fileprivate func ranked(_ rank: Int) -> Self {
        var row = self
        row.rank = Int32(rank)
        return row
    }
}
