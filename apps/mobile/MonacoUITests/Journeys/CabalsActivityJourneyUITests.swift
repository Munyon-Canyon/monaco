import XCTest

nonisolated final class CabalsActivityJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = CabalsActivityJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            try CabalsActivityJourney.readAndOpenReceipt(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-receipt")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            try CabalsActivityJourney.retryFailedTrade(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-failed-receipt")
        }
    }
}
