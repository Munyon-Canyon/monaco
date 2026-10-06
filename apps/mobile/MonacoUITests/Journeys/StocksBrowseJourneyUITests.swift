import XCTest

nonisolated final class StocksBrowseJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = StocksBrowseJourney.recorder()
        _ = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            StocksBrowseJourney.browseSections(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-all-stocks")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            try StocksBrowseJourney.search(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2-search-cleared")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            StocksBrowseJourney.pullToRefresh(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-refreshed")
        }
    }
}
