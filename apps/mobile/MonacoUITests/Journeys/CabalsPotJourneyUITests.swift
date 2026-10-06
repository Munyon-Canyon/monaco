import XCTest

nonisolated final class CabalsPotJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = CabalsPotJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            CabalsPotJourney.potAndSlice(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-pot")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            CabalsPotJourney.holdings(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-holdings")
        }
    }
}
