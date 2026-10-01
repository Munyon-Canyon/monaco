import Foundation
import MonacoCore
import MonacoTestClock
import Testing

@testable import Monaco

// The app target shadows these MonacoCore DTOs; pin the tests to the ones the views use.
private typealias MarketAssetDTO = Monaco.MarketAssetDTO
private typealias ListMarketAssetsResponse = Monaco.ListMarketAssetsResponse
private typealias PopularAssetsResponse = Monaco.PopularAssetsResponse
private typealias HeldAssetsResponse = Monaco.HeldAssetsResponse

@MainActor
private final class StubStocksDataSource: StocksTabDataSource {
    var searches: [(query: String, offset: Int)] = []
    var popularCalls = 0
    /// Per-query latency, so a slow page for an old query can land after a fast new one.
    var delays: [String: Duration] = [:]
    /// Per-offset latency, so page two can be held while page one answers at once.
    var offsetDelays: [Int: Duration] = [:]
    /// Per-offset failure, so page two can fail while the query's first page succeeds.
    var offsetErrors: [Int: Error] = [:]
    var errors: [String: Error] = [:]
    var popularError: Error?
    var heldCalls = 0
    var heldError: Error?
    var heldResponse = HeldAssetsResponse(held: [], upForVote: [])
    /// Rows served by `popular`, so a test can shape the mover strip.
    var popularAssets: [MarketAssetDTO] = [StubStocksDataSource.asset(symbol: "AAPLx")]
    var popularMarket: MarketStatusDTO?
    /// Search delays park here. A test advances it; this stub does not wait on the wall clock.
    let clock = TestClock()
    /// Incremented when `search` returns, so a test can wait for that hop.
    let searchesDone = Watched(0)

    func search(query: String, offset: Int, limit: Int) async throws -> ListMarketAssetsResponse {
        searches.append((query, offset))
        if let delay = offsetDelays[offset] ?? delays[query] {
            try? await clock.sleep(for: delay)
        }
        // A cancelled page still answers, so the model can drop it. It must not wake the
        // test that is waiting for the search that replaced it.
        if !Task.isCancelled {
            searchesDone.mutate { $0 += 1 }
        }
        if let error = offsetErrors[offset] ?? errors[query] { throw error }
        if query == "none" {
            return ListMarketAssetsResponse(assets: [], hasMore: false)
        }
        let assets = (0..<2).map { index in
            Self.asset(symbol: "\(query.uppercased())-\(offset + index)")
        }
        return ListMarketAssetsResponse(assets: assets, hasMore: true)
    }

    func popular(limit: Int) async throws -> PopularAssetsResponse {
        popularCalls += 1
        if let popularError { throw popularError }
        return PopularAssetsResponse(assets: popularAssets, market: popularMarket)
    }

    func held() async throws -> HeldAssetsResponse {
        heldCalls += 1
        if let heldError { throw heldError }
        return heldResponse
    }

    static func asset(
        symbol: String,
        change24h: String? = "0.012",
        spark: [Int64] = [180_000_000, 182_000_000, 185_000_000]
    ) -> MarketAssetDTO {
        MarketAssetDTO(
            symbol: symbol,
            name: "\(symbol) xStock",
            solanaMint: "Mint\(symbol)",
            routable: true,
            priceUsdcMicros: 185_000_000,
            change24h: change24h,
            sparkUsdcMicros: spark
        )
    }
}

@MainActor
struct StocksTabModelTests {
    private func make(
        _ source: StubStocksDataSource,
        now: @escaping () -> Date = Date.init
    ) -> StocksTabModel {
        StocksTabModel(dataSource: source, clock: now, sleepClock: source.clock)
    }

    /// Parks the debounce, moves the clock past it, then waits until that search returns.
    private func settle(_ source: StubStocksDataSource) async {
        let mark = source.searchesDone.current
        let slept = source.clock.state.current.requested.count
        let debounce = StocksTabModel.searchDebounce
        _ = await source.clock.state.until { state in
            state.requested.count > slept && state.pending >= 1
                && state.requested.last == debounce
        }
        source.clock.advance(by: StocksTabModel.searchDebounce)
        _ = await source.searchesDone.until { $0 > mark }
    }

    /// Waits until `count` sleeps are parked, so the next step runs while they are in flight.
    private func untilPending(_ clock: TestClock, _ count: Int) async {
        _ = await clock.state.until { $0.pending >= count }
    }

    @Test func typingQuicklySendsOnlyTheLastQuery() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.updateQuery("te")
        model.updateQuery("tes")
        model.updateQuery("tesla")
        await settle(source)

        #expect(source.searches.map(\.query) == ["tesla"])
        #expect(model.searchState == .results)
    }

    /// The bug: page two of "a" was appended to the list the user was reading for "tesla",
    /// taking the offset and the has-more flag with it.
    @Test func aLoadMorePageForAnOldQueryNeverLandsInTheNewList() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.updateQuery("a")
        await settle(source)
        #expect(model.results.count == 2)

        // Page two of "a" comes back long after "tesla" has replaced it on screen.
        source.delays["a"] = .milliseconds(700)
        let loadMore = Task { await model.loadMore() }
        await untilPending(source.clock, 1)
        model.updateQuery("tesla")
        await settle(source)
        source.clock.advance(by: .milliseconds(700))
        await loadMore.value

        #expect(model.trimmedQuery == "tesla")
        #expect(model.results.map(\.symbol) == ["TESLA-0", "TESLA-1"])
        #expect(model.searchState == .results)
    }

    /// The bug: the shared `defer` from a stale request cleared the loading flag mid-debounce,
    /// flashing "No matches for that search" over the query the user was still typing.
    @Test func aStalePageNeverFlashesAnEmptyOrFailedState() async throws {
        let source = StubStocksDataSource()
        let model = make(source)
        source.delays["a"] = .milliseconds(700)
        source.errors["a"] = Monaco.MonacoAPIError.httpStatus(500)

        model.updateQuery("a")
        await untilPending(source.clock, 1)
        source.clock.advance(by: .milliseconds(300))
        await untilPending(source.clock, 1)
        model.updateQuery("tesla")
        await settle(source)

        #expect(model.searchState == .results)
        #expect(model.results.map(\.symbol) == ["TESLA-0", "TESLA-1"])
    }

    @Test func loadMoreAppendsWithoutDuplicatingRows() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.updateQuery("tesla")
        await settle(source)
        await model.loadMore()

        #expect(model.results.count == 4)
        #expect(Set(model.results.map(\.symbol)).count == 4)
        #expect(source.searches.map(\.offset) == [0, 2])
    }

    @Test func aFailedRefreshKeepsTheRowsOnScreen() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.updateQuery("tesla")
        await settle(source)
        source.errors["tesla"] = Monaco.MonacoAPIError.httpStatus(500)
        await model.refreshSearch()

        #expect(model.searchState == .results)
        #expect(model.results.count == 2)
    }

    @Test func noMatchesShowsEmptyState() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.updateQuery("none")
        await settle(source)

        #expect(model.searchState == .empty)
        #expect(model.results.isEmpty)
    }

    @Test func serverFailureShowsRetryableError() async throws {
        let source = StubStocksDataSource()
        source.errors["tesla"] = Monaco.MonacoAPIError.httpStatus(500)
        let model = make(source)

        model.updateQuery("tesla")
        await settle(source)

        #expect(model.searchState == .failed)
        #expect(!model.sessionExpired)
    }

    @Test func rejectedSessionAsksTheViewToSignOut() async throws {
        let source = StubStocksDataSource()
        source.errors["tesla"] = Monaco.MonacoAPIError.httpStatus(401)
        let model = make(source)

        model.updateQuery("tesla")
        await settle(source)

        #expect(model.sessionExpired)
    }

    /// The bug: a missing token returned before the loading flag was cleared, so the tab
    /// showed "Loading stocks…" for ever.
    @Test func aMissingTokenEndsInAFailedState() async throws {
        let source = StubStocksDataSource()
        source.errors["tesla"] = Monaco.MonacoAPIError.missingAccessToken
        let model = make(source)

        model.updateQuery("tesla")
        await settle(source)

        #expect(model.searchState == .failed)
    }

    @Test func popularFailureIsRetryableInsteadOfLookingEmpty() async throws {
        let source = StubStocksDataSource()
        source.popularError = Monaco.MonacoAPIError.httpStatus(500)
        let model = make(source)

        await model.loadPopular()

        #expect(model.popularState == .failed)
        #expect(model.popular.isEmpty)
    }

    @Test func popularRefreshesOnlyOnceItIsStale() async throws {
        let source = StubStocksDataSource()
        var now = Date(timeIntervalSince1970: 1_000)
        let model = make(source, now: { now })

        await model.refreshPopularIfStale()
        await model.refreshPopularIfStale()
        #expect(source.popularCalls == 1)

        now = now.addingTimeInterval(StocksTabModel.popularStaleAfter)
        await model.refreshPopularIfStale()
        #expect(source.popularCalls == 2)
        #expect(model.popularState == .loaded)
    }

    /// The session's cached strip paints at once, and the prices are still refetched.
    @Test func seededPopularStillRefreshes() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.seedPopular([StubStocksDataSource.asset(symbol: "NVDAx")])
        #expect(model.popularState == .loaded)
        #expect(model.popular.map(\.symbol) == ["NVDAx"])

        await model.refreshPopularIfStale()
        #expect(source.popularCalls == 1)
        #expect(model.popular.map(\.symbol) == ["AAPLx"])
    }

    /// The review finding: the stale-page guard compared the query *text*, which is an ABA check.
    /// Typing an L and taking it off again leaves the same text on screen under a different
    /// search, and page two of the first one passed the guard and appended itself into the list
    /// the second one had just cleared.
    @Test func aPageForARetypedQueryNeverLandsInTheListThatReplacedIt() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.updateQuery("aap")
        await settle(source)
        #expect(model.results.map(\.symbol) == ["AAP-0", "AAP-1"])
        #expect(model.hasMore)

        // Page two of the "aap" on screen goes out, and is held in flight.
        source.offsetDelays[2] = .milliseconds(500)
        async let pageTwo: Void = model.loadMore()
        await untilPending(source.clock, 1)

        // The member types an L and deletes it: same text, a different search.
        model.updateQuery("aapl")
        model.updateQuery("aap")
        #expect(model.results.isEmpty)
        #expect(!model.isLoadingMore, "the new query's Load more must not be stuck on the old page")

        await settle(source)
        #expect(model.results.map(\.symbol) == ["AAP-0", "AAP-1"])
        #expect(model.hasMore)

        source.clock.advance(by: .milliseconds(500))
        await pageTwo
        #expect(
            !model.results.contains { $0.symbol == "AAP-2" },
            "page two of the retyped query must not land in the list that replaced it"
        )
        #expect(model.results.map(\.symbol) == ["AAP-0", "AAP-1"])
        #expect(model.hasMore)
    }

    /// The same orphaned page must not report *its* failure against the query that replaced it.
    @Test func aFailedPageForARetypedQueryDoesNotShowItsErrorOnTheNewOne() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.updateQuery("aap")
        await settle(source)

        // Page two is held and then fails; the retyped query's own first page still answers.
        source.offsetDelays[2] = .milliseconds(400)
        source.offsetErrors[2] = Monaco.MonacoAPIError.httpStatus(500)
        async let pageTwo: Void = model.loadMore()
        await untilPending(source.clock, 1)

        model.updateQuery("aapl")
        model.updateQuery("aap")
        await settle(source)
        #expect(model.searchState == .results)

        source.clock.advance(by: .milliseconds(400))
        await pageTwo

        #expect(!model.loadMoreFailed, "the old page's failure belongs to a query nobody is reading")
        #expect(model.searchState == .results)
    }

    /// `loadMoreFailed` drives the "Could not load more stocks." caption and nothing exercised it.
    @Test func aFailedLoadMoreKeepsTheRowsAndSaysSo() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.updateQuery("aap")
        await settle(source)
        #expect(model.results.count == 2)

        source.errors["aap"] = Monaco.MonacoAPIError.httpStatus(500)
        await model.loadMore()

        #expect(model.loadMoreFailed)
        #expect(model.results.count == 2, "the rows already read stay on screen")
        #expect(model.searchState == .results)
        #expect(!model.isLoadingMore)

        // Trying again clears the caption.
        source.errors["aap"] = nil
        await model.loadMore()
        #expect(!model.loadMoreFailed)
        #expect(model.results.map(\.symbol) == ["AAP-0", "AAP-1", "AAP-2", "AAP-3"])
    }

    /// A failed pull-to-refresh kept the rows but retracted the spinner in silence, so stale
    /// prices read as fresh ones.
    @Test func aFailedRefreshKeepsTheRowsAndAdmitsItFailed() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        model.updateQuery("aap")
        await settle(source)

        source.errors["aap"] = Monaco.MonacoAPIError.httpStatus(500)
        await model.refreshSearch()

        #expect(model.refreshFailed)
        #expect(model.results.map(\.symbol) == ["AAP-0", "AAP-1"])
        #expect(model.searchState == .results, "rows beat an error message")

        source.errors["aap"] = nil
        await model.refreshSearch()
        #expect(!model.refreshFailed)
    }

    // MARK: Sections

    @Test func rowsCarryTheirSparklineSoNoViewHasToBuildIt() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        await model.loadPopular()

        #expect(model.popularRows.map(\.id) == ["AAPLx"])
        #expect(model.popularRows.first?.spark != nil)
    }

    @Test func aSymbolWithNoDaySeriesStillMakesARow() async throws {
        let source = StubStocksDataSource()
        source.popularAssets = [StubStocksDataSource.asset(symbol: "NEWx", spark: [])]
        let model = make(source)

        await model.loadPopular()

        #expect(model.popularRows.count == 1)
        #expect(model.popularRows.first?.spark == nil, "no series means no line, not a flat one")
    }

    @Test func topMoversAreThePopularRowsResortedByTheDaysMove() async throws {
        let source = StubStocksDataSource()
        source.popularAssets = [
            StubStocksDataSource.asset(symbol: "SMALLx", change24h: "0.004"),
            StubStocksDataSource.asset(symbol: "DROPx", change24h: "-0.081"),
            StubStocksDataSource.asset(symbol: "MIDx", change24h: "0.030"),
            StubStocksDataSource.asset(symbol: "QUIETx", change24h: nil),
        ]
        let model = make(source)

        await model.loadPopular()

        #expect(model.moverRows.map(\.id) == ["DROPx", "MIDx", "SMALLx"])
        #expect(!model.moverRows.contains { $0.id == "QUIETx" }, "unknown is not a move")
    }

    @Test func theSessionOnTheEnvelopeReachesTheRows() async throws {
        let source = StubStocksDataSource()
        source.popularMarket = MarketSampleData.sessionAfterHours
        let model = make(source)

        await model.loadPopular()

        #expect(model.afterHours)
    }

    @Test func yourCabalsAndOpenVotesArriveTogether() async throws {
        let source = StubStocksDataSource()
        source.heldResponse = MarketSampleData.heldAssetsResponse()
        let model = make(source)

        await model.loadSocial()

        #expect(model.socialState == .loaded)
        #expect(model.heldRows.count == MarketSampleData.heldAssets.count)
        #expect(model.voteRows.count == MarketSampleData.votableAssets.count)
        #expect(model.heldRows.first?.subtitle == "2 cabals · your slice $294.70")
        #expect(model.voteRows.first?.subtitle == "1 open vote · Semis or bust")
    }

    @Test func cabalsThatOwnNothingIsAnAnswerNotAFailure() async throws {
        let source = StubStocksDataSource()
        let model = make(source)

        await model.loadSocial()

        #expect(model.socialState == .loaded)
        #expect(model.heldRows.isEmpty)
    }

    @Test func aFailedCabalReadIsItsOwnFailureAndLeavesTheCatalogueAlone() async throws {
        let source = StubStocksDataSource()
        source.heldError = Monaco.MonacoAPIError.httpStatus(500)
        let model = make(source)

        await model.refreshEverything()

        #expect(model.socialState == .failed)
        #expect(model.popularState == .loaded, "the market is still live")
        #expect(!model.popularRows.isEmpty)
    }

    @Test func cabalRowsAlreadyOnScreenSurviveAFailedRefresh() async throws {
        let source = StubStocksDataSource()
        source.heldResponse = MarketSampleData.heldAssetsResponse()
        let model = make(source)
        await model.loadSocial()

        source.heldError = Monaco.MonacoAPIError.httpStatus(500)
        await model.loadSocial()

        #expect(model.socialState == .loaded, "a stale holding beats an empty section")
        #expect(!model.heldRows.isEmpty)
    }

    /// The rows staying is right; the rows staying *silently* is not. The figure on
    /// them is "your slice $294.70", and nothing else on screen said it was old.
    @Test func aFailedCabalRefreshMarksTheRowsStale() async throws {
        let source = StubStocksDataSource()
        source.heldResponse = MarketSampleData.heldAssetsResponse()
        let model = make(source)
        await model.loadSocial()
        #expect(!model.socialRefreshFailed)

        source.heldError = Monaco.MonacoAPIError.httpStatus(500)
        await model.loadSocial()

        #expect(model.socialRefreshFailed, "a member must be told the money on screen is not fresh")
        #expect(!model.heldRows.isEmpty)
    }

    @Test func aSuccessfulRefreshClearsTheStaleMark() async throws {
        let source = StubStocksDataSource()
        source.heldResponse = MarketSampleData.heldAssetsResponse()
        let model = make(source)
        await model.loadSocial()
        source.heldError = Monaco.MonacoAPIError.httpStatus(500)
        await model.loadSocial()
        #expect(model.socialRefreshFailed)

        source.heldError = nil
        await model.loadSocial()

        #expect(!model.socialRefreshFailed)
        #expect(model.socialState == .loaded)
    }

    /// An empty section that failed is `.failed`, which has its own retry. It is not
    /// also stale — there is nothing on screen to be stale.
    @Test func aFirstCabalReadThatFailsIsNotStaleItIsFailed() async throws {
        let source = StubStocksDataSource()
        source.heldError = Monaco.MonacoAPIError.httpStatus(500)
        let model = make(source)

        await model.loadSocial()

        #expect(model.socialState == .failed)
        #expect(!model.socialRefreshFailed)
    }

    @Test func anExpiredSessionFromTheCabalReadIsReported() async throws {
        let source = StubStocksDataSource()
        source.heldError = Monaco.MonacoAPIError.httpStatus(401)
        let model = make(source)

        await model.loadSocial()

        #expect(model.sessionExpired)
        #expect(model.socialState == .loading, "a dead session is not an empty cabal list")
    }

    @Test func aRefreshWithinTheStaleWindowDoesNotReAskForTheCabals() async throws {
        let now = Date()
        let source = StubStocksDataSource()
        let model = make(source, now: { now })

        await model.refreshSocialIfStale()
        await model.refreshSocialIfStale()

        #expect(source.heldCalls == 1)
    }
}
