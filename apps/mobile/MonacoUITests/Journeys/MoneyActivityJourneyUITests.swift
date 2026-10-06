import XCTest

nonisolated final class MoneyActivityJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = MoneyActivityJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            let txn = try JourneyHandoff.read(MoneyActivityJourney.depositKey)
            try session.act(as: "A")
            MoneyActivityJourney.depositReceipt(app, txn: txn, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-activity")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            MoneyActivityJourney.cabalMove(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-cabal")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            MoneyActivityJourney.withdrawal(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-withdrawal")
        }
    }
}
