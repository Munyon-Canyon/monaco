import Foundation
import Testing

@testable import Monaco

@MainActor
struct HomeScreenStateTests {
    private func dashboard(netWorthUsd: String = "1248.50") -> HomeDashboardDTO {
        HomeDashboardDTO(
            netWorthUsd: netWorthUsd,
            netWorthDollarPnl: "+48.20",
            netWorthPercentReturn: "0.040",
            myGroups: [],
            pnlSeries1H: [],
            leaderboard: HomeLeaderboardSectionDTO(range: "ALL", people: [])
        )
    }

    @Test func nothingLoadedYetIsLoading() {
        #expect(HomeScreenState.resolve(dashboard: nil, errorMessage: nil) == .loading)
    }

    @Test func aFailedFirstLoadShowsTheFailure() {
        #expect(HomeScreenState.resolve(dashboard: nil, errorMessage: "No connection.") == .failed("No connection."))
    }

    /// The old ladder had an unreachable branch that rendered a made-up "$0.00" dashboard.
    /// Every combination now resolves to a state built from real data.
    @Test func aLoadedBoardIsNeverReplacedByAFabricatedZero() {
        let loaded = dashboard()
        #expect(HomeScreenState.resolve(dashboard: loaded, errorMessage: nil) == .loaded(loaded))
        #expect(HomeScreenState.resolve(dashboard: loaded, errorMessage: "No connection.") == .loaded(loaded))
    }
}

@MainActor
struct HomeHeroChartTests {
    private func points(_ count: Int) -> [HomePnLSeriesPointDTO] {
        (0..<count).map { index in
            HomePnLSeriesPointDTO(
                ts: Date(timeIntervalSince1970: TimeInterval(index * 300)),
                equityUsd: "1000.00",
                dollarPnl: "+\(index).00"
            )
        }
    }

    @Test func aMemberWithNoCabalsGetsNoSlot() {
        #expect(HomeHeroChart.resolve(loaded: nil, embedded: [], hasCabals: false) == .hidden)
        #expect(HomeHeroChart.resolve(loaded: points(5), embedded: [], hasCabals: false) == .hidden)
    }

    /// The slot is the same height before, during and after the series lands, so Home does
    /// not shift when the curve arrives — or when it turns out to be too short to draw.
    @Test func theSlotIsHeldFromTheFirstFrameUntilTheCurveCanBeDrawn() {
        #expect(HomeHeroChart.resolve(loaded: nil, embedded: [], hasCabals: true) == .reserved(hasResolved: false))
        #expect(HomeHeroChart.resolve(loaded: points(2), embedded: [], hasCabals: true) == .reserved(hasResolved: true))
        #expect(HomeHeroChart.resolve(loaded: points(1), embedded: [], hasCabals: true) == .reserved(hasResolved: true))
    }

    /// The cold-start case. The backend hard-codes the dashboard's own `pnlSeries1H` to an
    /// empty array and the real series arrives on a later read, so "nothing yet" and "came
    /// back with one point" have to be different answers — otherwise the hero tells every
    /// member with a cabal "No curve yet" on every launch and takes it back a second later.
    @Test func aSeriesThatHasNotComeBackIsNotASeriesThatCameBackEmpty() {
        let pending = HomeHeroChart.resolve(loaded: nil, embedded: [], hasCabals: true)
        let answered = HomeHeroChart.resolve(loaded: [], embedded: [], hasCabals: true)

        #expect(pending == .reserved(hasResolved: false))
        #expect(answered == .reserved(hasResolved: true))
        #expect(pending != answered)
    }

    /// The dashboard's embedded copy is empty on every real read, so it is only an answer
    /// when it carries points — otherwise it would resolve the pending case for us.
    @Test func theDashboardsEmptyCopyDoesNotCountAsAnAnswer() {
        #expect(HomeHeroChart.resolve(loaded: nil, embedded: [], hasCabals: true) == .reserved(hasResolved: false))

        let embedded = points(4)
        #expect(HomeHeroChart.resolve(loaded: nil, embedded: embedded, hasCabals: true) == .curve(embedded))
    }

    @Test func threePointsEarnTheCurve() {
        let series = points(3)
        #expect(HomeHeroChart.resolve(loaded: series, embedded: [], hasCabals: true) == .curve(series))
    }

    /// The separate 1H read wins over the dashboard's stale copy once it lands.
    @Test func theLoadedSeriesWinsOverTheDashboardsCopy() {
        let loaded = points(5)
        #expect(HomeHeroChart.resolve(loaded: loaded, embedded: points(3), hasCabals: true) == .curve(loaded))
    }
}
