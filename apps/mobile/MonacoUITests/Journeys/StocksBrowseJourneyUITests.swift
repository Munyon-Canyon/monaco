import XCTest

nonisolated final class StocksBrowseJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func start() throws -> XCUIApplication {
        let account = try JourneyAccount.load()
        _ = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        return app
    }

    @MainActor
    func testS1BrowseSections() throws {
        let app = try start()
        StocksBrowseJourney.browseSections(app, recorder: StocksBrowseJourney.recorder())
        attachScreenshot(of: app, named: "S1-all-stocks")
    }

    @MainActor
    func testS2Search() throws {
        let app = try start()
        try StocksBrowseJourney.search(app, recorder: StocksBrowseJourney.recorder())
        attachScreenshot(of: app, named: "S2-search-cleared")
    }

    @MainActor
    func testS3PullToRefresh() throws {
        let app = try start()
        StocksBrowseJourney.pullToRefresh(app, recorder: StocksBrowseJourney.recorder())
        attachScreenshot(of: app, named: "S3-refreshed")
    }
}
