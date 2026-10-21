import MonacoAPI
import MonacoTestSupport
import Observation
import XCTest

@testable import MonacoCore

@MainActor
final class LeaderboardPageModelTests: XCTestCase {
    private typealias Entry = LeaderboardFixture.Entry

    private func entry(
        _ rank: Int32, _ name: String, pnl: Int64 = 1_000_000, members: Int = 8,
        movement: LeaderboardMovement? = .up(1), membership: LeaderboardMembership = .open, delayed: Bool = false
    ) -> Entry {
        Entry(
            row: LeaderboardRowView(
                .sample(
                    rank: rank, id: name.lowercased(), name: name, kind: .cabal, pnlMicros: pnl,
                    flags: delayed ? [.stalePrices] : [])),
            annotation: LeaderboardAnnotation(movement: movement, memberCount: members, membership: membership))
    }

    private func model(
        board: LeaderboardBoardKind = .cabals, range: LeaderboardRange = .oneMonth,
        filters: LeaderboardFilters = .standard, fixture: LeaderboardFixture? = nil
    ) -> LeaderboardPageModel {
        LeaderboardPageModel(board: board, range: range, filters: filters, fixture: fixture)
    }

    private var cabals: [Entry] {
        [
            entry(1, "Ape", pnl: 5, members: 12, movement: .up(4)),
            entry(2, "Quant", pnl: 9, members: 4, movement: .down(1), membership: .approval),
            entry(3, "Dip", pnl: 7, members: 7, movement: .unchanged),
        ]
    }

    func testContextLineNamesTheBoardAndPeriod() async {
        XCTAssertEqual(
            model(fixture: LeaderboardFixture(cabals: cabals)).contextLine,
            "Cabal P&L · 1M · Rank change vs previous snapshot")
        XCTAssertEqual(
            model(board: .people, range: .all, fixture: LeaderboardFixture()).contextLine,
            "Shared personal P&L · All · Rank change vs previous snapshot")
    }

    func testLiveContextLineSaysMovementOpensSoon() async {
        XCTAssertEqual(model().contextLine, "Cabal P&L · 1M · Rank change opens soon")
        XCTAssertEqual(
            model(board: .people, range: .oneWeek).contextLine, "Shared personal P&L · 1W · Rank change opens soon")
    }

    func testTheContextLineFollowsTheCategoryAndPeriod() async {
        let page = model(fixture: LeaderboardFixture())

        page.select(category: .people)
        page.select(range: .oneDay)

        XCTAssertEqual(page.contextLine, "Shared personal P&L · 1D · Rank change vs previous snapshot")
    }

    func testABoardIsStaleOnlyWhenEveryRowIsDelayed() async {
        let delayed = [entry(1, "A", delayed: true), entry(2, "B", delayed: true)]
        let mixed = [entry(1, "A", delayed: true), entry(2, "B")]

        XCTAssertEqual(model(fixture: LeaderboardFixture(cabals: delayed)).pageState, .stale)
        XCTAssertEqual(model(fixture: LeaderboardFixture(cabals: mixed)).pageState, .rows)
        XCTAssertTrue(LeaderboardPageModel.isStale(delayed.map(\.row)))
        XCTAssertFalse(LeaderboardPageModel.isStale(mixed.map(\.row)))
        XCTAssertFalse(LeaderboardPageModel.isStale([]))
    }

    func testNoRowsIsEmptyAndFilteredOutRowsAreFilteredEmpty() async {
        XCTAssertEqual(model(fixture: LeaderboardFixture()).pageState, .empty)

        var filters = LeaderboardFilters.standard
        filters.minimumMembers = .ten
        let page = model(filters: filters, fixture: LeaderboardFixture(cabals: [entry(1, "Tiny", members: 3)]))

        XCTAssertEqual(page.pageState, .filteredEmpty)
        page.resetFilters()
        XCTAssertEqual(page.pageState, .rows)
    }

    func testFixturePhasesMapToTheLoadingAndFailedStates() async {
        XCTAssertEqual(model(fixture: LeaderboardFixture(phase: .loading)).pageState, .loading)
        XCTAssertEqual(model(fixture: LeaderboardFixture(phase: .failed)).pageState, .failed)
    }

    func testApplyCommitsTheDraft() async {
        let page = model(fixture: LeaderboardFixture(cabals: cabals))
        page.beginEditingFilters()
        page.draftFilters.membership = .approval

        XCTAssertEqual(page.appliedFilters, .standard)
        page.applyDraft()

        XCTAssertEqual(page.appliedFilters.membership, .approval)
        XCTAssertTrue(page.hasFiltersApplied)
        XCTAssertEqual(page.entries.map(\.row.name), ["Quant"])
    }

    func testDiscardDropsTheDraft() async {
        let page = model(fixture: LeaderboardFixture(cabals: cabals))
        page.beginEditingFilters()
        page.draftFilters.sort = .name

        page.discardDraft()

        XCTAssertEqual(page.draftFilters, .standard)
        XCTAssertEqual(page.appliedFilters, .standard)
        XCTAssertEqual(page.entries.map(\.row.name), ["Ape", "Quant", "Dip"])
    }

    func testResetRestoresAndAppliesTheDefaults() async {
        var filters = LeaderboardFilters.standard
        filters.sort = .pnlAmount
        let page = model(filters: filters, fixture: LeaderboardFixture(cabals: cabals))
        XCTAssertEqual(page.entries.map(\.row.name), ["Quant", "Dip", "Ape"])

        page.resetFilters()

        XCTAssertEqual(page.appliedFilters, .standard)
        XCTAssertEqual(page.draftFilters, .standard)
        XCTAssertEqual(page.entries.map(\.row.name), ["Ape", "Quant", "Dip"])
    }

    func testMovementShowsOnlyForTheReturnSort() async {
        let page = model(fixture: LeaderboardFixture(cabals: cabals))
        XCTAssertEqual(page.entries.map(\.glyphMovement), [4, -1, 0])

        page.beginEditingFilters()
        page.draftFilters.sort = .name
        page.applyDraft()

        XCTAssertEqual(page.entries.map(\.glyphMovement), [0, 0, 0])
        XCTAssertTrue(page.entries.allSatisfy { !$0.comparesMovement })
    }

    func testANewRowHasNoGlyphAndLiveRowsCarryNone() async {
        let page = model(fixture: LeaderboardFixture(cabals: [entry(1, "Fresh", movement: .new)]))

        XCTAssertNil(page.entries.first?.glyphMovement)
        XCTAssertEqual(LeaderboardCopy.spoken(.new), "New on the board")
    }

    func testTheSubLineCountsMembers() async {
        let page = model(fixture: LeaderboardFixture(cabals: [entry(1, "One", members: 1), entry(2, "Many")]))

        XCTAssertEqual(page.entries.map(\.subtitle), ["1 member", "8 members"])
    }

    func testMovementIsSpokenInPlaces() async {
        XCTAssertEqual(LeaderboardCopy.spoken(.up(4)), "Up 4 places from the previous snapshot")
        XCTAssertEqual(LeaderboardCopy.spoken(.down(1)), "Down 1 place from the previous snapshot")
        XCTAssertEqual(LeaderboardCopy.spoken(.unchanged), "Unchanged")
    }

    func testTheFooterNamesTheBoard() async {
        XCTAssertEqual(LeaderboardCopy.allShown(board: .cabals, count: 5), "All 5 eligible cabals shown")
        XCTAssertEqual(LeaderboardCopy.allShown(board: .people, count: 2), "All 2 eligible people shown")
    }
}

@MainActor
final class LeaderboardPageModelLiveTests: XCTestCase {
    private typealias Page = Components.Schemas.LeaderboardPage

    private func reply(_ page: Page) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(page), as: UTF8.self))
    }

    private func live(
        _ script: [StubTransport.Reply], board: LeaderboardBoardKind = .cabals
    ) -> (LeaderboardPageModel, StubTransport) {
        let transport = StubTransport(scripted: script)
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let hints = FakeHintStream()
        let model = LeaderboardPageModel(board: board) { kind, range, filters in
            LeaderboardLoader(
                board: kind, range: range, filter: kind == .people ? filters.scope.filter : .everyone,
                filters: filters, api: api, hints: hints)
        }
        return (model, transport)
    }

    private func delayed(_ rank: Int32, _ id: String, _ stale: Bool) -> Components.Schemas.LeaderboardRow {
        .sample(rank: rank, id: id, name: id, kind: .cabal, flags: stale ? [.stalePrices] : [])
    }

    private func page(_ rows: [Components.Schemas.LeaderboardRow], next: String? = nil) -> Page {
        var page = Page.samplePeople(count: 0)
        page.rows = rows
        page.nextCursor = next
        return page
    }

    func testALiveBoardLoadsRowsWithoutMovement() async throws {
        let (model, transport) = live([try reply(.samplePeople())], board: .people)

        XCTAssertEqual(model.pageState, .loading)
        await model.reload()
        let loader = try XCTUnwrap(model.liveLoader)
        await loader.load()

        XCTAssertEqual(model.pageState, .rows)
        XCTAssertEqual(model.entries.count, 3)
        XCTAssertTrue(model.entries.allSatisfy { $0.glyphMovement == nil })
        XCTAssertNil(model.pinnedMe)
        XCTAssertFalse(model.hasMore)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.first, "/v1/leaderboards/people?range=1M&filter=all")
    }

    func testEveryDelayedRowMakesTheLiveBoardStale() async throws {
        let rows = [delayed(1, "a", true), delayed(2, "b", true)]
        let (model, _) = live([try reply(page(rows))])
        await model.liveLoader?.load()

        XCTAssertEqual(model.pageState, .stale)
    }

    func testSomeDelayedRowsKeepTheRows() async throws {
        let rows = [delayed(1, "a", true), delayed(2, "b", false)]
        let (model, _) = live([try reply(page(rows))])
        await model.liveLoader?.load()

        XCTAssertEqual(model.pageState, .rows)
    }

    func testAnEmptyLiveBoardIsEmpty() async throws {
        let (model, _) = live([try reply(.sampleEmpty)])
        await model.liveLoader?.load()

        XCTAssertEqual(model.pageState, .empty)
        XCTAssertFalse(model.followingIsEmpty)
    }

    func testFollowingNobodyIsTheFollowEmptyState() async throws {
        let (model, transport) = live(
            [try reply(.samplePeople(count: 2)), try reply(.sampleEmpty)], board: .people)
        await model.liveLoader?.load()
        model.beginEditingFilters()
        model.draftFilters.scope = .following

        model.applyDraft()
        let settled = await eventArrives(within: 60) { await Self.becomes { model.pageState == .empty } }

        XCTAssertTrue(settled)
        XCTAssertTrue(model.followingIsEmpty)
        XCTAssertEqual(model.pageState, .empty)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/leaderboards/people?range=1M&filter=friends")
    }

    private static func becomes(_ predicate: @MainActor () -> Bool) async {
        while !predicate() {
            await withCheckedContinuation { (continuation: CheckedContinuation<Void, Never>) in
                withObservationTracking {
                    _ = predicate()
                } onChange: {
                    continuation.resume()
                }
            }
        }
    }

    private func waitForRequests(_ count: Int, on transport: StubTransport) async {
        for _ in 0..<5_000 {
            if await transport.sent.count >= count { return }
            await Task.yield()
        }
    }

    func testTheCabalFiltersAreSentOnApplyAndDroppedOnReset() async throws {
        let (model, transport) = live(
            [try reply(.samplePeople()), try reply(.samplePeople()), try reply(.samplePeople())])
        await model.liveLoader?.load()
        model.beginEditingFilters()
        model.draftFilters.sort = .pnlAmount
        model.draftFilters.membership = .acceptingRequests
        model.draftFilters.minimumMembers = .ten

        model.applyDraft()
        await waitForRequests(2, on: transport)
        XCTAssertTrue(model.hasFiltersApplied)
        model.resetFilters()
        await waitForRequests(3, on: transport)

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(
            paths,
            [
                "/v1/leaderboards/cabals?range=1M",
                "/v1/leaderboards/cabals?range=1M&sort=pnl&membership=accepting_requests&min_members=10",
                "/v1/leaderboards/cabals?range=1M",
            ])
    }

    func testThePeopleSortIsSentAndTheCabalOnlyFiltersAreNot() async throws {
        let (model, transport) = live([try reply(.samplePeople()), try reply(.samplePeople())], board: .people)
        await model.liveLoader?.load()
        model.beginEditingFilters()
        model.draftFilters.sort = .name
        model.draftFilters.membership = .open

        model.applyDraft()
        await waitForRequests(2, on: transport)

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/leaderboards/people?range=1M&filter=all&sort=name")
    }

    func testAFilterThatMatchesNothingIsTheFilteredEmptyState() async throws {
        let (model, transport) = live([try reply(.samplePeople()), try reply(.sampleEmpty)])
        await model.liveLoader?.load()
        model.beginEditingFilters()
        model.draftFilters.minimumMembers = .ten

        model.applyDraft()
        await waitForRequests(2, on: transport)
        for _ in 0..<5_000 where model.pageState != .filteredEmpty { await Task.yield() }

        XCTAssertEqual(model.pageState, .filteredEmpty)
    }

    func testAFailedFirstLoadIsFailedAndRetryReads() async throws {
        let (model, _) = live([.failure(URLError(.notConnectedToInternet)), try reply(.samplePeople())])
        await model.liveLoader?.load()
        XCTAssertEqual(model.pageState, .failed)

        model.retry()
        for _ in 0..<5_000 where model.pageState != .rows { await Task.yield() }

        XCTAssertEqual(model.pageState, .rows)
    }

    func testSwitchingCategoryKeepsEachBoardsRows() async throws {
        let (model, transport) = live([try reply(page([delayed(1, "ape", false)])), try reply(.samplePeople())])
        await model.liveLoader?.load()
        model.select(category: .people)
        await model.liveLoader?.load()
        XCTAssertEqual(model.entries.count, 3)

        model.select(category: .cabals)

        XCTAssertEqual(model.entries.map(\.id), ["ape"])
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testTheLoaderPagesThroughLoadMore() async throws {
        let first = page([delayed(1, "a", false)], next: "c2")
        let second = page([delayed(2, "b", false)])
        let (model, _) = live([try reply(first), try reply(second)])
        await model.liveLoader?.load()
        XCTAssertTrue(model.hasMore)

        await model.loadMore()

        XCTAssertEqual(model.entries.map(\.id), ["a", "b"])
        XCTAssertFalse(model.hasMore)
        XCTAssertFalse(model.isLoadingMore)
    }

    func testAPeriodChangeReloadsTheBoard() async throws {
        let (model, transport) = live([try reply(.samplePeople()), try reply(.samplePeople())], board: .people)
        await model.liveLoader?.load()

        model.select(range: .all)
        for _ in 0..<5_000 {
            if await transport.sent.count == 2 { break }
            await Task.yield()
        }

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/leaderboards/people?range=ALL&filter=all")
        XCTAssertEqual(model.range, .all)
    }

    func testReloadSetVisibleAndToastPassThrough() async throws {
        let (model, _) = live(
            [try reply(.samplePeople()), .failure(URLError(.notConnectedToInternet))], board: .people)
        await model.liveLoader?.load()

        model.setVisible(true)
        await model.reload()

        XCTAssertNotNil(model.toast)
        model.dismissToast()
        XCTAssertNil(model.toast)
        XCTAssertEqual(model.pageState, .rows)
    }
}
