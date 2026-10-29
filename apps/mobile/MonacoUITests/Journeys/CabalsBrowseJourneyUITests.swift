import XCTest

nonisolated final class CabalsBrowseJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = CabalsBrowseJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            CabalsBrowseJourney.browses(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-requested")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            CabalsBrowseJourney.holdsATile(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-menu")
        }
    }
}
