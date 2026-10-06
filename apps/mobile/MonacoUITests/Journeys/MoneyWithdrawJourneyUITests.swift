import XCTest

nonisolated final class MoneyWithdrawJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = MoneyWithdrawJourney.recorder()
        let address = try MoneyWithdrawJourney.refundAddress()

        try session.scenario("S1") {
            try session.act(as: "A")
            MoneyWithdrawJourney.withdrawAll(app, to: address, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-withdrawn")
        }
    }
}
