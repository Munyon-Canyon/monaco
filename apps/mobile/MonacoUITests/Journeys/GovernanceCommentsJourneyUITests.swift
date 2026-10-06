import XCTest

nonisolated final class GovernanceCommentsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = GovernanceCommentsJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            GovernanceCommentsJourney.readAndPost(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-posted")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            GovernanceCommentsJourney.reply(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-replied")
        }
    }
}
