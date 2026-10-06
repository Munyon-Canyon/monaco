import XCTest

nonisolated final class CabalsPauseJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = CabalsPauseJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            CabalsPauseJourney.pausedShowsWhy(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-paused")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            CabalsPauseJourney.runningShowsNothing(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-running")
        }
    }
}
