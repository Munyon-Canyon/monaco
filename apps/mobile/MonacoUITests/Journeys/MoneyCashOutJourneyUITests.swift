import XCTest

nonisolated final class MoneyCashOutJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = MoneyCashOutJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            MoneyCashOutJourney.cashOut(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-cashed-out")
        }

        try session.scenario("S2") {
            guard let address = ProcessInfo.processInfo.environment["MONACO_QA_REFUND_ADDRESS"], !address.isEmpty
            else {
                XCTFail("no {QA.refund_address}: scripts/qa/journey.py passes MONACO_QA_REFUND_ADDRESS")
                return
            }
            try session.act(as: "A")
            MoneyCashOutJourney.withdraw(app, refundAddress: address, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-withdrawing")
        }
    }
}
