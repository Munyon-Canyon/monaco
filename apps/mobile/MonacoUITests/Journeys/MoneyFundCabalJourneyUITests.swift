import XCTest

nonisolated final class MoneyFundCabalJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = MoneyFundCabalJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            MoneyFundCabalJourney.pickAmount(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-amount")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            MoneyFundCabalJourney.addToPot(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-funded")
        }
    }
}
