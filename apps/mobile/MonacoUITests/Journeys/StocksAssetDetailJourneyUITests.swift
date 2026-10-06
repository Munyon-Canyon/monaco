import XCTest

nonisolated final class StocksAssetDetailJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = StocksAssetDetailJourney.recorder()
        _ = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            StocksAssetDetailJourney.heroChartAndBuy(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-propose-buy")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            StocksAssetDetailJourney.stats(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2-stats")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            StocksAssetDetailJourney.preIpoBlock(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-pre-ipo")
        }

        try session.scenario("S4") {
            try session.act(as: "A")
            StocksAssetDetailJourney.position(app, recorder: recorder)
            attachScreenshot(of: app, named: "S4-position")
        }
    }
}
