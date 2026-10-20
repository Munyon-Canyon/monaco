import XCTest

nonisolated final class StocksWatchlistJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = StocksWatchlistJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            StocksWatchlistJourney.starFromManage(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-watchlist-starred")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            StocksWatchlistJourney.removeFromStock(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-watchlist-emptied")
        }
    }
}
