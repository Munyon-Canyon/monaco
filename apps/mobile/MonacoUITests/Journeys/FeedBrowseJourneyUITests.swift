import XCTest

nonisolated final class FeedBrowseJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = FeedBrowseJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            FeedBrowseJourney.chipsAndSearch(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-search")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            FeedBrowseJourney.following(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-following")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            FeedBrowseJourney.cellTaps(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-cells")
        }
    }
}
