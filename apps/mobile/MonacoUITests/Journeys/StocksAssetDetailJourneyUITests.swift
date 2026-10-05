import XCTest

nonisolated final class StocksAssetDetailJourneyUITests: XCTestCase {
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
    func testS1HeroChartAndBuy() throws {
        let app = try start()
        StocksAssetDetailJourney.heroChartAndBuy(app, run: try JourneyRun.id(), recorder: StocksAssetDetailJourney.recorder())
        attachScreenshot(of: app, named: "S1-propose-buy")
    }

    @MainActor
    func testS2Stats() throws {
        let app = try start()
        StocksAssetDetailJourney.stats(app, recorder: StocksAssetDetailJourney.recorder())
        attachScreenshot(of: app, named: "S2-stats")
    }

    @MainActor
    func testS3PreIpo() throws {
        let app = try start()
        StocksAssetDetailJourney.preIpoBlock(app, recorder: StocksAssetDetailJourney.recorder())
        attachScreenshot(of: app, named: "S3-pre-ipo")
    }

    @MainActor
    func testS4Position() throws {
        let app = try start()
        StocksAssetDetailJourney.position(app, recorder: StocksAssetDetailJourney.recorder())
        attachScreenshot(of: app, named: "S4-position")
    }
}
