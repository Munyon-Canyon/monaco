import XCTest

nonisolated final class HomeActivityJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = HomeActivityJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            HomeActivityJourney.opensFromBell(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-activity")
        }
    }
}
