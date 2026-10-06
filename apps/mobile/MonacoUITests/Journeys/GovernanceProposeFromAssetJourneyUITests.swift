import XCTest

nonisolated final class GovernanceProposeFromAssetJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = GovernanceProposeFromAssetJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            GovernanceProposeFromAssetJourney.proposeFromAsset(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-proposal-sent")
        }
    }
}
